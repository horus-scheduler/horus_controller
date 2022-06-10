package net

import (
	"context"
	"errors"
	"log"
	"net"
	"sync"
	"time"

	"github.com/golang/protobuf/ptypes/empty"
	"github.com/khaledmdiab/horus_controller/core"
	"github.com/khaledmdiab/horus_controller/core/model"
	horus_pb "github.com/khaledmdiab/horus_controller/protobuf"
	grpcpool "github.com/processout/grpc-go-pool"
	"github.com/sirupsen/logrus"
	"google.golang.org/grpc"
)

type SpineRpcEndpoint struct {
	sync.RWMutex
	topoLAddr         string
	vcAddress         string
	topology          *model.Topology
	vcm               *core.VCManager
	failedLeavesEgExt chan *LeafFailedMessage
	failedLeavesEgInt chan *LeafFailedMessage
	vcCentralConnPool *grpcpool.Pool
	mgConnPool        map[string]*grpcpool.Pool
	doneChan          chan bool
}

func NewSpineRpcEndpoint(topoLAddr, vcAddress string,
	topology *model.Topology,
	vcm *core.VCManager,
	failedLeaves chan *LeafFailedMessage) *SpineRpcEndpoint {
	return &SpineRpcEndpoint{
		topoLAddr:         topoLAddr,
		vcAddress:         vcAddress,
		topology:          topology,
		vcm:               vcm,
		failedLeavesEgExt: failedLeaves,
		failedLeavesEgInt: make(chan *LeafFailedMessage, DefaultRpcRecvSize),
		mgConnPool:        make(map[string]*grpcpool.Pool),
		doneChan:          make(chan bool, 1),
	}
}

func (s *SpineRpcEndpoint) createTopoListener() error {
	lis, err := net.Listen("tcp4", s.topoLAddr)
	if err != nil {
		log.Println(err)
		return err
	}
	rpcServer := grpc.NewServer()
	topoServer := NewSpineTopologyServer(s.topology, s.vcm, s.failedLeavesEgExt, s.failedLeavesEgInt)
	horus_pb.RegisterHorusTopologyServer(rpcServer, topoServer)
	return rpcServer.Serve(lis)
}

func (s *SpineRpcEndpoint) createVCConnPool() {
	s.Lock()
	defer s.Unlock()
	// Used for contacting the centralized controller
	var factory grpcpool.Factory
	factory = func() (*grpc.ClientConn, error) {
		conn, err := grpc.Dial(s.vcAddress, grpc.WithInsecure())
		if err != nil {
			logrus.Error(err)
		}
		return conn, err
	}
	var err error
	s.vcCentralConnPool, err = grpcpool.New(factory, 10, 20, 5*time.Second)
	if err != nil {
		logrus.Error(err)
	}
}

func (s *SpineRpcEndpoint) createMgrConnPool() {
	// s.Lock()
	// defer s.Unlock()

	for _, leaf := range s.topology.Leaves.Internal() {
		addr := leaf.MgmtAddress
		logrus.Debug("XXX ", addr)
		if _, found := s.mgConnPool[addr]; !found {
			var factory grpcpool.Factory
			factory = func() (*grpc.ClientConn, error) {
				conn, err := grpc.Dial(addr, grpc.WithInsecure())
				if err != nil {
					logrus.Error(err)
				}
				return conn, err
			}
			var err error
			s.mgConnPool[addr], err = grpcpool.New(factory, 10, 20, 5*time.Second)
			if err != nil {
				logrus.Error(err)
			}
		}
	}
}

func (s *SpineRpcEndpoint) GetVCs() ([]*horus_pb.VCInfo, error) {
	s.RLock()
	defer s.RUnlock()
	conn, err := s.vcCentralConnPool.Get(context.Background())
	if err != nil {
		logrus.Error(err)
		return nil, err
	}
	defer conn.Close()
	client := horus_pb.NewHorusVCClient(conn.ClientConn)
	logrus.Debug("SENDING")
	resp, err := client.GetVCs(context.Background(), &empty.Empty{})
	logrus.Debug("RECEIVED")
	return resp.Vcs, err
}

func (s *SpineRpcEndpoint) sendLeafFailed(failed *LeafFailedMessage) error {
	if failed != nil && failed.Dsts != nil {
		logrus.Debug("Mgmt Address: ", failed.Dsts[0].MgmtAddress)
		if connPool, found := s.mgConnPool[failed.Dsts[0].MgmtAddress]; !found {
			return errors.New("connection pool isn't found")
		} else {
			conn, err := connPool.Get(context.Background())
			if err != nil {
				logrus.Error(err)
				return err
			}
			defer conn.Close()
			client := horus_pb.NewHorusTopologyClient(conn.ClientConn)
			logrus.Debug("sending fail leaf to manager")
			_, err = client.FailLeaf(context.Background(), failed.Leaf)
			logrus.Debug("returned from sending fail leaf to manager")
			return err
		}
	}
	return errors.New("failed leaf doesn't exist")
}

func (s *SpineRpcEndpoint) processEvents() {
	for {
		select {
		case failed := <-s.failedLeavesEgInt:
			logrus.Debug("fail leaf...")
			logrus.Debug("sendLeafFailed: ", failed.Leaf.Id)
			err := s.sendLeafFailed(failed)
			if err != nil {
				logrus.Warn(err)
			}
			// s.Lock()
			logrus.Debug("send failed leaf to failedLeavesEgExt: ", failed.Leaf.Id)
			s.failedLeavesEgExt <- NewLeafFailedMessage(failed.Leaf, nil)
			// s.Unlock()
		default:
			continue
		}
	}
}

func (s *SpineRpcEndpoint) Start() {
	// create a pool of gRPC connections.
	s.createVCConnPool()
	s.createMgrConnPool()

	// creates the end point server.
	go s.createTopoListener()

	// Let's send any outstanding events
	go s.processEvents()
	<-s.doneChan
}
