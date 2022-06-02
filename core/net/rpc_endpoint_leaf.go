package net

import (
	"context"
	"sync"
	"time"

	"github.com/golang/protobuf/ptypes/empty"
	horus_pb "github.com/khaledmdiab/horus_controller/protobuf"
	grpcpool "github.com/processout/grpc-go-pool"
	"github.com/sirupsen/logrus"
	"google.golang.org/grpc"
)

type LeafRpcEndpoint struct {
	sync.RWMutex
	topoAddress string
	vcAddress   string

	vcConnPool *grpcpool.Pool
	doneChan   chan bool
}

func NewLeafRpcEndpoint(topoAddress, vcAddress string) *LeafRpcEndpoint {
	return &LeafRpcEndpoint{
		topoAddress: topoAddress,
		vcAddress:   vcAddress,
		doneChan:    make(chan bool, 1),
	}
}

func (s *LeafRpcEndpoint) createListener() {
	// lis, err := net.Listen("tcp4", s.lAddress)
	// if err != nil {
	// 	log.Println(err)
	// }
	// rpcServer := grpc.NewServer()
	// updater := NewUpdateServer(s.)
	// horus_pb.RegisterMdcSessionUpdaterServer(rpcServer, updater)
	// if err := rpcServer.Serve(lis); err != nil {
	// 	log.Println("Failed to start Hello Server", err)
	// }
}

func (s *LeafRpcEndpoint) createVCConnPool() {
	s.Lock()
	defer s.Unlock()
	var factory grpcpool.Factory
	factory = func() (*grpc.ClientConn, error) {
		conn, err := grpc.Dial(s.vcAddress, grpc.WithInsecure())
		if err != nil {
			logrus.Error(err)
		}
		return conn, err
	}
	var err error
	s.vcConnPool, err = grpcpool.New(factory, 10, 20, 5*time.Second)
	if err != nil {
		logrus.Error(err)
	}
}

func (s *LeafRpcEndpoint) GetVCs() ([]*horus_pb.VCInfo, error) {
	s.RLock()
	defer s.RUnlock()
	conn, err := s.vcConnPool.Get(context.Background())
	if err != nil {
		logrus.Error(err)
		return nil, err
	}
	defer conn.Close()
	client := horus_pb.NewHorusVCClient(conn.ClientConn)
	resp, err := client.GetVCs(context.Background(), &empty.Empty{})
	return resp.Vcs, err
}

func (s *LeafRpcEndpoint) processEvents() {
	// for {
	// 	select {
	// 	case syncEv := <-s.syncEvents:
	// 		err := s.sendSyncEvent(syncEv)
	// 		if err != nil {
	// 			log.Println(err)
	// 		}
	// 	default:
	// 		continue
	// 	}
	// }
}

func (s *LeafRpcEndpoint) Start() {
	// create a pool of gRPC connections. used to send SyncEvent messages
	s.createVCConnPool()

	// creates the end point server. used to recv SessionUpdateEvent messages
	go s.createListener()

	// Let's send any outstanding events
	go s.processEvents()

	<-s.doneChan
}
