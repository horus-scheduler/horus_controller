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

	failedLeaves chan *LeafFailedMessage

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
	topoServer := NewCentralTopologyServer(s.topology, s.vcm, s.failedLeaves)
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
	if ok {
		conn, err := pool.Get(context.Background())
		if err != nil {
			log.Println(err)
			return err
		}
		defer conn.Close()
		logrus.Debugf("Sending FailLeaf to Spine: %d", dst.ID)
		client := horus_pb.NewHorusTopologyClient(conn.ClientConn)
		_, err = client.FailLeaf(context.Background(), e)
		return err
	}
	return errors.New("Spine ID " + strconv.Itoa(int(dst.ID)) + " doesn't exist!")
}

func (s *CentralRpcEndpoint) processEvents() {
	for {
		select {
		case failedLeaf := <-s.failedLeaves:
			for _, dst := range failedLeaf.Dsts {
				err := s.sendFailedLeafEvent(failedLeaf.Leaf, dst)
				if err != nil {
					log.Println(err)
				}
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
