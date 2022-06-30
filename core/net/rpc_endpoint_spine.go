package net

import (
	"context"
	"errors"
	"log"
	"net"
	"strconv"
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
	SrvLAddr       string
	SrvCentralAddr string

	topology *model.Topology
	vcm      *core.VCManager

	failedLeavesEgExt chan *LeafFailedMessage
	failedLeavesEgInt chan *LeafFailedMessage

	failedServersEgExt chan *ServerFailedMessage
	failedServersEgInt chan *ServerFailedMessage

	newLeavesEgExt chan *LeafAddedMessage
	newLeavesEgInt chan *LeafAddedMessage

	newServersEgExt chan *ServerAddedMessage
	newServersEgInt chan *ServerAddedMessage

	newVCsEgExt chan *VCUpdatedMessage
	newVCsEgInt chan *VCUpdatedMessage

	centralConnPool *grpcpool.Pool
	mgConnPool      map[string]*grpcpool.Pool
	leafConnPool    map[string]*grpcpool.Pool
	doneChan        chan bool
}

func NewSpineRpcEndpoint(srvLAddr, srvCentralAddr string,
	topology *model.Topology,
	vcm *core.VCManager,
	failedLeaves chan *LeafFailedMessage,
	failedServers chan *ServerFailedMessage,
	newLeaves chan *LeafAddedMessage,
	newServers chan *ServerAddedMessage,
	newVCs chan *VCUpdatedMessage,
) *SpineRpcEndpoint {
	return &SpineRpcEndpoint{
		SrvLAddr:           srvLAddr,
		SrvCentralAddr:     srvCentralAddr,
		topology:           topology,
		vcm:                vcm,
		failedLeavesEgExt:  failedLeaves,
		failedLeavesEgInt:  make(chan *LeafFailedMessage, DefaultRpcRecvSize),
		failedServersEgExt: failedServers,
		failedServersEgInt: make(chan *ServerFailedMessage, DefaultRpcRecvSize),
		newLeavesEgExt:     newLeaves,
		newLeavesEgInt:     make(chan *LeafAddedMessage, DefaultRpcRecvSize),
		newServersEgExt:    newServers,
		newServersEgInt:    make(chan *ServerAddedMessage, DefaultRpcRecvSize),
		newVCsEgExt:        newVCs,
		newVCsEgInt:        make(chan *VCUpdatedMessage, DefaultRpcRecvSize),
		mgConnPool:         make(map[string]*grpcpool.Pool),
		leafConnPool:       make(map[string]*grpcpool.Pool),
		doneChan:           make(chan bool, 1),
	}
}

func (s *SpineRpcEndpoint) createServiceListener() error {
	lis, err := net.Listen("tcp4", s.SrvLAddr)
	if err != nil {
		log.Println(err)
		return err
	}
	rpcServer := grpc.NewServer()
	topoServer := NewSpineSrvServer(s.topology, s.vcm,
		s.failedLeavesEgInt,
		s.failedServersEgInt,
		s.newLeavesEgInt,
		s.newServersEgInt,
		s.newVCsEgInt)
	horus_pb.RegisterHorusServiceServer(rpcServer, topoServer)
	return rpcServer.Serve(lis)
}

func (s *SpineRpcEndpoint) createCentralConnPool() {
	s.Lock()
	defer s.Unlock()
	// Used for contacting the centralized controller
	logrus.Infof("[SpineRPC] Adding centralized address (%s) to the connection pool", s.SrvCentralAddr)
	var factory grpcpool.Factory
	factory = func() (*grpc.ClientConn, error) {
		conn, err := grpc.Dial(s.SrvCentralAddr, grpc.WithInsecure())
		if err != nil {
			logrus.Error(err)
		}
		return conn, err
	}
	var err error
	s.centralConnPool, err = grpcpool.New(factory, 10, 20, 5*time.Second)
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
	conn, err := s.centralConnPool.Get(context.Background())
	if err != nil {
		logrus.Error(err)
		return nil, err
	}
	defer conn.Close()
	client := horus_pb.NewHorusServiceClient(conn.ClientConn)
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
			client := horus_pb.NewHorusServiceClient(conn.ClientConn)
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
			client := horus_pb.NewHorusServiceClient(conn.ClientConn)
			logrus.Debug("[SpineRPC] Sending fail server to leaf")
			_, err = client.FailServer(context.Background(), failed.Server)
			logrus.Debug("[SpineRPC] Returned from sending fail server to leaf")
			return err
		}
	}
	return errors.New("failed leaf doesn't exist")
}

func (s *SpineRpcEndpoint) sendLeafAdded(newLeaf *LeafAddedMessage) error {
	if newLeaf.Leaf != nil && newLeaf.Dst != nil {
		addr := newLeaf.Dst.MgmtAddress
		logrus.Debugf("[SpineRPC] Mgmt Address: %s", addr)
		if connPool, found := s.mgConnPool[addr]; !found {
			return errors.New("connection pool isn't found")
		} else {
			conn, err := connPool.Get(context.Background())
			if err != nil {
				logrus.Error(err)
				return err
			}
			defer conn.Close()
			client := horus_pb.NewHorusServiceClient(conn.ClientConn)
			logrus.Debug("[SpineRPC] Sending add leaf to manager")
			_, err = client.AddLeaf(context.Background(), newLeaf.Leaf)
			logrus.Debug("[SpineRPC] Returned from sending add leaf to manager")
			return err
		}
	}
	return errors.New("added leaf doesn't exist")
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
			client := horus_pb.NewHorusServiceClient(conn.ClientConn)
			logrus.Debug("[SpineRPC] Sending add server to leaf")
			_, err = client.AddServer(context.Background(), newServer.Server)
			logrus.Debug("[SpineRPC] Returned from sending add server to leaf")
			return err
		}
	}
	return errors.New("added server doesn't exist")
}

func (s *SpineRpcEndpoint) sendAddVCToLeaf(vcInfo *horus_pb.VCInfo, vcType VCUpdateType, leaf *model.Node) error {
	if leaf == nil {
		return errors.New("leaf doesn't exist")
	}

	pool, ok := s.leafConnPool[leaf.Address]
	if !ok {
		return errors.New("the pool for leaf " + strconv.Itoa(int(leaf.ID)) + " doesn't exist")
	}

	conn, err := pool.Get(context.Background())
	if err != nil {
		logrus.Error(err)
		return err
	}
	defer conn.Close()

	client := horus_pb.NewHorusServiceClient(conn.ClientConn)

	if vcType == VCUpdateAdd {
		logrus.Debugf("[SpineRPC] Sending AddVC to Leaf %d", leaf.ID)
		_, err = client.AddVC(context.Background(), vcInfo)
	} else if vcType == VCUpdateRem {
		logrus.Debugf("[SpineRPC] Sending RemoveVC to Leaf %d", leaf.ID)
		_, err = client.RemoveVC(context.Background(), vcInfo)
	}
	return err
}

func (s *SpineRpcEndpoint) broadcastAddVCToLeaves(msg *VCUpdatedMessage) error {
	leaves := msg.Dsts
	if len(leaves) == 0 {
		return errors.New("leaves don't exist")
	}
	currentVCInfo := msg.VCInfo
	for _, leaf := range leaves {
		vcInfo := &horus_pb.VCInfo{
			Id:     currentVCInfo.Id,
			Spines: currentVCInfo.Spines,
		}
		for _, serverInfo := range currentVCInfo.Servers {
			server := s.topology.GetNode(uint16(serverInfo.Id), model.NodeType_Server)
			if server != nil && server.Parent != nil && server.Parent.ID == leaf.ID {
				serverInfo := &horus_pb.VCServerInfo{}
				serverInfo.Id = uint32(server.ID)
				vcInfo.Servers = append(vcInfo.Servers, serverInfo)
			}
		}

		err := s.sendAddVCToLeaf(vcInfo, msg.Type, leaf)
		if err != nil {
			return err
		}
	}
	return nil
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
		case newLeaf := <-s.newLeavesEgInt:
			logrus.Debug("[SpineRPC] Add leaf...")
			logrus.Debugf("[SpineRPC] Calling sendServerAdded: %d", newLeaf.Leaf.Id)
			s.createLeafConnPool()
			err := s.sendLeafAdded(newLeaf)
			if err != nil {
				logrus.Warn(err)
			}
			logrus.Debugf("[SpineRPC] Send added leaf %d to newLeavesEgExt", newLeaf.Leaf.Id)
			s.newLeavesEgExt <- NewLeafAddedMessage(newLeaf.Leaf, newLeaf.Dst)
		case newServer := <-s.newServersEgInt:
			logrus.Debug("[SpineRPC] Add server...")
			logrus.Debugf("[SpineRPC] Calling sendServerAdded: %d", newServer.Server.Id)
			err := s.sendServerAdded(newServer)
			if err != nil {
				logrus.Warn(err)
			}
			logrus.Debugf("[SpineRPC] Send added server %d to newServersEgExt", newServer.Server.Id)
			s.newServersEgExt <- NewServerAddedMessage(newServer.Server, newServer.Dst)
		case newVC := <-s.newVCsEgInt:
			err := s.broadcastAddVCToLeaves(newVC)
			if err != nil {
				logrus.Error(err)
			}
			if newVC.Type == VCUpdateAdd {
				logrus.Debugf("[SpineRPC] Send added VC %d to newVCsEgExt", newVC.VCInfo.Id)
			} else if newVC.Type == VCUpdateRem {
				logrus.Debugf("[SpineRPC] Send removed VC %d to newVCsEgExt", newVC.VCInfo.Id)
			}
			s.newVCsEgExt <- NewVCUpdatedMessage(newVC.VCInfo, newVC.Type, newVC.Dsts)
		default:
			continue
		}
	}
}

func (s *SpineRpcEndpoint) Start() {
	// create a pool of gRPC connections.
	s.createCentralConnPool()
	s.createMgrConnPool()
	s.createLeafConnPool()

	// creates the end point server.
	go s.createServiceListener()

	// Let's send any outstanding events
	go s.processEvents()
	<-s.doneChan
}
