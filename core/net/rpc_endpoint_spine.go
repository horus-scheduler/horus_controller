package net

import (
	"log"
	"net"

	"github.com/khaledmdiab/horus_controller/core"
	"github.com/khaledmdiab/horus_controller/core/model"
	horus_pb "github.com/khaledmdiab/horus_controller/protobuf"
	"google.golang.org/grpc"
)

type SpineRpcEndpoint struct {
	topoLAddr    string
	topology     *model.Topology
	vcm          *core.VCManager
	failedLeaves chan *LeafFailedMessage
	// connPool *grpcpool.Pool
	doneChan chan bool
}

func NewSpineRpcEndpoint(topoLAddr string,
	topology *model.Topology,
	vcm *core.VCManager,
	failedLeaves chan *LeafFailedMessage) *SpineRpcEndpoint {
	return &SpineRpcEndpoint{
		topoLAddr:    topoLAddr,
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

func (s *SpineRpcEndpoint) createConnPool() {
	// var factory grpcpool.Factory
	// factory = func() (*grpc.ClientConn, error) {
	// 	conn, err := grpc.Dial(s.rAddress, grpc.WithInsecure())
	// 	if err != nil {
	// 		log.Println(err)
	// 	}
	// 	return conn, err
	// }
	// var err error
	// s.connPool, err = grpcpool.New(factory, 10, 20, 5*time.Second)
	// if err != nil {
	// 	log.Println(err)
	// }
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
	s.createConnPool()

	// creates the end point server. used to recv SessionUpdateEvent messages
	go s.createTopoListener()

	// Let's send any outstanding events
	go s.processEvents()
	<-s.doneChan
}
