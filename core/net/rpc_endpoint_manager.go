package net

import (
	"log"
	"net"
	"sync"

	horus_pb "github.com/khaledmdiab/horus_controller/protobuf"
	"google.golang.org/grpc"
)

type ManagerRpcEndpoint struct {
	sync.RWMutex
	mgmtAddress  string
	failedLeaves chan *LeafFailedMessage
	newLeaves    chan *LeafAddedMessage
	doneChan     chan bool
}

func NewManagerRpcEndpoint(mgmtAddress string,
	failedLeaves chan *LeafFailedMessage,
	newLeaves chan *LeafAddedMessage,
) *ManagerRpcEndpoint {
	return &ManagerRpcEndpoint{
		mgmtAddress:  mgmtAddress,
		failedLeaves: failedLeaves,
		newLeaves:    newLeaves,
		doneChan:     make(chan bool, 1),
	}
}

func (s *ManagerRpcEndpoint) createTopoListener() error {
	lis, err := net.Listen("tcp4", s.mgmtAddress)
	if err != nil {
		log.Println(err)
		return err
	}
	rpcServer := grpc.NewServer()
	topoServer := NewManagerTopologyServer(s.failedLeaves, s.newLeaves)
	horus_pb.RegisterHorusTopologyServer(rpcServer, topoServer)
	return rpcServer.Serve(lis)
}

func (s *ManagerRpcEndpoint) processEvents() {
}

func (s *ManagerRpcEndpoint) Start() {
	// creates the end point server
	go s.createTopoListener()

	// Let's send any outstanding events
	go s.processEvents()

	<-s.doneChan
}
