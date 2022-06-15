package net

import (
	"context"
	"errors"
	"log"
	"net"
	"strconv"
	"time"

	"github.com/khaledmdiab/horus_controller/core"
	"github.com/khaledmdiab/horus_controller/core/model"
	horus_pb "github.com/khaledmdiab/horus_controller/protobuf"
	grpcpool "github.com/processout/grpc-go-pool"
	"github.com/sirupsen/logrus"
	"google.golang.org/grpc"
)

type CentralRpcEndpoint struct {
	vcLAddr   string
	topoLAddr string

	failedLeaves  chan *LeafFailedMessage
	failedServers chan *ServerFailedMessage
	newServers    chan *ServerAddedMessage

	topology      *model.Topology
	vcm           *core.VCManager
	spineConnPool map[uint16]*grpcpool.Pool

	doneChan chan bool
}

func NewCentralRpcEndpoint(topoLAddr, vcLAddr string,
	topology *model.Topology,
	vcm *core.VCManager,
) *CentralRpcEndpoint {
	connPool := make(map[uint16]*grpcpool.Pool)
	return &CentralRpcEndpoint{
		topoLAddr:     topoLAddr,
		vcLAddr:       vcLAddr,
		topology:      topology,
		vcm:           vcm,
		spineConnPool: connPool,
		failedLeaves:  make(chan *LeafFailedMessage, DefaultRpcRecvSize),
		failedServers: make(chan *ServerFailedMessage, DefaultRpcRecvSize),
		newServers:    make(chan *ServerAddedMessage, DefaultRpcRecvSize),
		doneChan:      make(chan bool, 1),
	}
}

func (s *CentralRpcEndpoint) createVCListener() error {
	lis, err := net.Listen("tcp4", s.vcLAddr)
	if err != nil {
		log.Println(err)
		return err
	}
	rpcServer := grpc.NewServer()
	vcServer := NewCentralVCServer(s.vcm)
	horus_pb.RegisterHorusVCServer(rpcServer, vcServer)
	return rpcServer.Serve(lis)
}

func (s *CentralRpcEndpoint) createTopoListener() error {
	lis, err := net.Listen("tcp4", s.topoLAddr)
	if err != nil {
		log.Println(err)
		return err
	}
	rpcServer := grpc.NewServer()
	topoServer := NewCentralTopologyServer(s.topology, s.vcm, s.failedLeaves, s.failedServers, s.newServers)
	horus_pb.RegisterHorusTopologyServer(rpcServer, topoServer)
	return rpcServer.Serve(lis)
}

func (s *CentralRpcEndpoint) createSpineConnPool(spineAddr string) *grpcpool.Pool {
	var factory grpcpool.Factory
	factory = func() (*grpc.ClientConn, error) {
		conn, err := grpc.Dial(spineAddr, grpc.WithInsecure())
		if err != nil {
			log.Println(err)
		}
		return conn, err
	}
	var err error
	connPool, err := grpcpool.New(factory, 2, 6, 5*time.Second)
	if err != nil {
		log.Println(err)
	}
	return connPool
}

func (s *CentralRpcEndpoint) sendFailedLeafEvent(e *horus_pb.LeafInfo, dst *model.Node) error {
	pool, ok := s.spineConnPool[dst.ID]
	if !ok {
		return errors.New("Spine ID " + strconv.Itoa(int(dst.ID)) + " doesn't exist!")
	}

	conn, err := pool.Get(context.Background())
	if err != nil {
		logrus.Error(err)
		return err
	}
	defer conn.Close()
	logrus.Debugf("[CentralRPC] Sending FailLeaf to Spine: %d", dst.ID)
	client := horus_pb.NewHorusTopologyClient(conn.ClientConn)
	_, err = client.FailLeaf(context.Background(), e)
	return err
}

func (s *CentralRpcEndpoint) sendFailedServerEvent(e *horus_pb.ServerInfo, dst *model.Node) error {
	pool, ok := s.spineConnPool[dst.ID]
	if !ok {
		return errors.New("spine ID " + strconv.Itoa(int(dst.ID)) + " doesn't exist!")
	}
	conn, err := pool.Get(context.Background())
	if err != nil {
		logrus.Error(err)
		return err
	}
	defer conn.Close()
	logrus.Debugf("[CentralRPC] Sending FailServer to Spine: %d", dst.ID)
	client := horus_pb.NewHorusTopologyClient(conn.ClientConn)
	_, err = client.FailServer(context.Background(), e)
	return err
}

func (s *CentralRpcEndpoint) sendAddServerEvent(msg *ServerAddedMessage) error {
	spine := msg.Dst
	if spine == nil {
		return errors.New("spine doesn't exist")
	}
	pool, ok := s.spineConnPool[spine.ID]
	if !ok {
		return errors.New("the pool for spine " + strconv.Itoa(int(spine.ID)) + " doesn't exist")
	}
	conn, err := pool.Get(context.Background())
	if err != nil {
		logrus.Error(err)
		return err
	}
	defer conn.Close()
	logrus.Debugf("[CentralRPC] Sending AddServer to Spine %d", spine.ID)
	client := horus_pb.NewHorusTopologyClient(conn.ClientConn)
	_, err = client.AddServer(context.Background(), msg.Server)
	return err
}

func (s *CentralRpcEndpoint) processEvents() {
	for {
		select {
		case failedLeaf := <-s.failedLeaves:
			for _, dst := range failedLeaf.Dsts {
				err := s.sendFailedLeafEvent(failedLeaf.Leaf, dst)
				if err != nil {
					logrus.Error(err)
				}
			}
		case failedServer := <-s.failedServers:
			for _, dst := range failedServer.Dsts {
				err := s.sendFailedServerEvent(failedServer.Server, dst)
				if err != nil {
					logrus.Error(err)
				}
			}
		case newServerMsg := <-s.newServers:
			err := s.sendAddServerEvent(newServerMsg)
			if err != nil {
				logrus.Error(err)
			}

		default:
			continue
		}
	}
}

func (s *CentralRpcEndpoint) Start() {
	// create an array of connection pools of gRPC clients.
	// each gRPC client corresponds to a Spine controller.
	for _, spine := range s.topology.Spines.Internal() {
		connPool := s.createSpineConnPool(spine.Address)
		s.spineConnPool[spine.ID] = connPool
	}

	// creates the end point servers for both App and ToR
	go s.createTopoListener()
	go s.createVCListener()

	// Let's send any outstanding events
	go s.processEvents()
	<-s.doneChan
}
