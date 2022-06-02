package net

import (
	"context"
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
	topoLAddr    string
	vcAddress    string
	topology     *model.Topology
	vcm          *core.VCManager
	failedLeaves chan *LeafFailedMessage
	vcConnPool   *grpcpool.Pool
	doneChan     chan bool
}

func NewSpineRpcEndpoint(topoLAddr, vcAddress string,
	topology *model.Topology,
	vcm *core.VCManager,
	failedLeaves chan *LeafFailedMessage) *SpineRpcEndpoint {
	return &SpineRpcEndpoint{
		topoLAddr:    topoLAddr,
		vcAddress:    vcAddress,
		topology:     topology,
		vcm:          vcm,
		failedLeaves: failedLeaves,
		doneChan:     make(chan bool, 1),
	}
}

func (s *SpineRpcEndpoint) createTopoListener() error {
	lis, err := net.Listen("tcp4", s.topoLAddr)
	if err != nil {
		log.Println(err)
		return err
	}
	rpcServer := grpc.NewServer()
	topoServer := NewSpineTopologyServer(s.topology, s.vcm, s.failedLeaves)
	horus_pb.RegisterHorusTopologyServer(rpcServer, topoServer)
	return rpcServer.Serve(lis)
}

func (s *SpineRpcEndpoint) createVCConnPool() {
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

func (s *SpineRpcEndpoint) GetVCs() ([]*horus_pb.VCInfo, error) {
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

func (s *SpineRpcEndpoint) processEvents() {
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

func (s *SpineRpcEndpoint) Start() {
	// create a pool of gRPC connections. used to send SyncEvent messages
	s.createVCConnPool()

	// creates the end point server. used to recv SessionUpdateEvent messages
	go s.createTopoListener()

	// Let's send any outstanding events
	go s.processEvents()
	<-s.doneChan
}
