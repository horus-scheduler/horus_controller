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
	TopoLAddr         string
	VCAddress         string
	topology          *model.Topology
	vcm               *core.VCManager
	failedLeavesEgExt chan *LeafFailedMessage
	failedLeavesEgInt chan *LeafFailedMessage

	failedServersEgExt chan *ServerFailedMessage
	failedServersEgInt chan *ServerFailedMessage

	newServersEgExt chan *ServerAddedMessage
	newServersEgInt chan *ServerAddedMessage

	vcCentralConnPool *grpcpool.Pool
	mgConnPool        map[string]*grpcpool.Pool
	leafConnPool      map[string]*grpcpool.Pool
	doneChan          chan bool
}

func NewSpineRpcEndpoint(topoLAddr, vcAddress string,
	topology *model.Topology,
	vcm *core.VCManager,
	failedLeaves chan *LeafFailedMessage,
	failedServers chan *ServerFailedMessage,
	newServers chan *ServerAddedMessage,
) *SpineRpcEndpoint {
	return &SpineRpcEndpoint{
		TopoLAddr:          topoLAddr,
		VCAddress:          vcAddress,
		topology:           topology,
		vcm:                vcm,
		failedLeavesEgExt:  failedLeaves,
		failedLeavesEgInt:  make(chan *LeafFailedMessage, DefaultRpcRecvSize),
		failedServersEgExt: failedServers,
		failedServersEgInt: make(chan *ServerFailedMessage, DefaultRpcRecvSize),
		newServersEgExt:    newServers,
		newServersEgInt:    make(chan *ServerAddedMessage, DefaultRpcRecvSize),
		mgConnPool:         make(map[string]*grpcpool.Pool),
		leafConnPool:       make(map[string]*grpcpool.Pool),
		doneChan:           make(chan bool, 1),
	}
}

func (s *SpineRpcEndpoint) createTopoListener() error {
	lis, err := net.Listen("tcp4", s.TopoLAddr)
	if err != nil {
		log.Println(err)
		return err
	}
	rpcServer := grpc.NewServer()
	topoServer := NewSpineTopologyServer(s.topology, s.vcm,
		s.failedLeavesEgInt, s.failedServersEgInt, s.newServersEgInt)
	horus_pb.RegisterHorusTopologyServer(rpcServer, topoServer)
	return rpcServer.Serve(lis)
}

func (s *SpineRpcEndpoint) createVCConnPool() {
	s.Lock()
	defer s.Unlock()
	// Used for contacting the centralized controller
	logrus.Infof("[SpineRPC] Adding centralized VC address (%s) to the connection pool", s.VCAddress)
	var factory grpcpool.Factory
	factory = func() (*grpc.ClientConn, error) {
		conn, err := grpc.Dial(s.VCAddress, grpc.WithInsecure())
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
	for _, leaf := range s.topology.Leaves.Internal() {
		addr := leaf.MgmtAddress
		if _, found := s.mgConnPool[addr]; !found {
			logrus.Infof("[SpineRPC] Adding leaf management address (%s) to the connection pool", addr)
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

func (s *SpineRpcEndpoint) createLeafConnPool() {
	for _, leaf := range s.topology.Leaves.Internal() {
		addr := leaf.Address
		if _, found := s.leafConnPool[addr]; !found {
			logrus.Infof("[SpineRPC] Adding leaf address (%s) to the connection pool", addr)
			var factory grpcpool.Factory
			factory = func() (*grpc.ClientConn, error) {
				conn, err := grpc.Dial(addr, grpc.WithInsecure())
				if err != nil {
					logrus.Error(err)
				}
				return conn, err
			}
			var err error
			s.leafConnPool[addr], err = grpcpool.New(factory, 10, 20, 5*time.Second)
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
	logrus.Debug("[SpineRPC] Sending GetVCs")
	resp, err := client.GetVCs(context.Background(), &empty.Empty{})
	logrus.Debug("[SpineRPC] Received GetVCs")
	return resp.Vcs, err
}

func (s *SpineRpcEndpoint) sendLeafFailed(failed *LeafFailedMessage) error {
	if failed != nil && failed.Dsts != nil {
		addr := failed.Dsts[0].MgmtAddress
		logrus.Debugf("[SpineRPC] Leaf Mgmt Address: %s", addr)
		if connPool, found := s.mgConnPool[addr]; !found {
			return errors.New("connection pool isn't found")
		} else {
			conn, err := connPool.Get(context.Background())
			if err != nil {
				logrus.Error(err)
				return err
			}
			defer conn.Close()
			client := horus_pb.NewHorusTopologyClient(conn.ClientConn)
			logrus.Debug("[SpineRPC] Sending fail leaf to manager")
			_, err = client.FailLeaf(context.Background(), failed.Leaf)
			logrus.Debug("[SpineRPC] Returned from sending fail leaf to manager")
			return err
		}
	}
	return errors.New("failed leaf doesn't exist")
}

func (s *SpineRpcEndpoint) sendServerFailed(failed *ServerFailedMessage) error {
	if failed != nil && failed.Dsts != nil {
		addr := failed.Dsts[0].Address
		logrus.Debugf("[SpineRPC] Leaf Address: %s", addr)
		if connPool, found := s.leafConnPool[addr]; !found {
			return errors.New("connection pool isn't found")
		} else {
			conn, err := connPool.Get(context.Background())
			if err != nil {
				logrus.Error(err)
				return err
			}
			defer conn.Close()
			client := horus_pb.NewHorusTopologyClient(conn.ClientConn)
			logrus.Debug("[SpineRPC] Sending fail server to leaf")
			_, err = client.FailServer(context.Background(), failed.Server)
			logrus.Debug("[SpineRPC] Returned from sending fail server to leaf")
			return err
		}
	}
	return errors.New("failed leaf doesn't exist")
}

func (s *SpineRpcEndpoint) sendServerAdded(newServer *ServerAddedMessage) error {
	if newServer.Server != nil && newServer.Dst != nil {
		addr := newServer.Dst.Address
		logrus.Debugf("[SpineRPC] Leaf Address: %s", addr)
		if connPool, found := s.leafConnPool[addr]; !found {
			return errors.New("connection pool isn't found")
		} else {
			conn, err := connPool.Get(context.Background())
			if err != nil {
				logrus.Error(err)
				return err
			}
			defer conn.Close()
			client := horus_pb.NewHorusTopologyClient(conn.ClientConn)
			logrus.Debug("[SpineRPC] Sending add server to leaf")
			_, err = client.AddServer(context.Background(), newServer.Server)
			logrus.Debug("[SpineRPC] Returned from sending add server to leaf")
			return err
		}
	}
	return errors.New("added server doesn't exist")
}

func (s *SpineRpcEndpoint) processEvents() {
	for {
		select {
		case failed := <-s.failedLeavesEgInt:
			logrus.Debug("[SpineRPC] Fail leaf...")
			logrus.Debug("[SpineRPC] Calling sendLeafFailed: ", failed.Leaf.Id)
			err := s.sendLeafFailed(failed)
			if err != nil {
				logrus.Warn(err)
			}
			logrus.Debugf("[SpineRPC] Send failed leaf %d to failedLeavesEgExt", failed.Leaf.Id)
			s.failedLeavesEgExt <- NewLeafFailedMessage(failed.Leaf, nil)
		case failed := <-s.failedServersEgInt:
			logrus.Debug("[SpineRPC] Fail server...")
			logrus.Debugf("[SpineRPC] Calling sendServerFailed: %d", failed.Server.Id)
			err := s.sendServerFailed(failed)
			if err != nil {
				logrus.Warn(err)
			}
			logrus.Debugf("[SpineRPC] Send failed %d server to failedServersEgExt", failed.Server.Id)
			s.failedServersEgExt <- NewServerFailedMessage(failed.Server, nil)
		case newServer := <-s.newServersEgInt:
			logrus.Debug("[SpineRPC] Add server...")
			logrus.Debugf("[SpineRPC] Calling sendServerAdded: %d", newServer.Server.Id)
			err := s.sendServerAdded(newServer)
			if err != nil {
				logrus.Warn(err)
			}
			logrus.Debugf("[SpineRPC] Send added server %d to newServersEgExt", newServer.Server.Id)
			s.newServersEgExt <- NewServerAddedMessage(newServer.Server, newServer.Dst)
		default:
			continue
		}
	}
}

func (s *SpineRpcEndpoint) Start() {
	// create a pool of gRPC connections.
	s.createVCConnPool()
	s.createMgrConnPool()
	s.createLeafConnPool()

	// creates the end point server.
	go s.createTopoListener()

	// Let's send any outstanding events
	go s.processEvents()
	<-s.doneChan
}
