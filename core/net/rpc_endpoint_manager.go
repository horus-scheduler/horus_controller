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
	srvLAddress  string
	failedLeaves chan *LeafFailedMessage
	newLeaves    chan *LeafAddedMessage
	doneChan     chan bool
}

func NewManagerRpcEndpoint(srvLAddress string,
	failedLeaves chan *LeafFailedMessage,
	newLeaves chan *LeafAddedMessage,
) *ManagerRpcEndpoint {
	return &ManagerRpcEndpoint{
		srvLAddress:  srvLAddress,
		failedLeaves: failedLeaves,
		newLeaves:    newLeaves,
		doneChan:     make(chan bool, 1),
	}
}

func (s *ManagerRpcEndpoint) createServiceListener() error {
	lis, err := net.Listen("tcp4", s.srvLAddress)
	if err != nil {
		log.Println(err)
		return err
	}
	rpcServer := grpc.NewServer()
	topoServer := NewManagerSrvServer(s.failedLeaves, s.newLeaves)
	horus_pb.RegisterHorusServiceServer(rpcServer, topoServer)
	return rpcServer.Serve(lis)
}

func (s *ManagerRpcEndpoint) processEvents() {
}

func (s *ManagerRpcEndpoint) Start() {
	// creates the end point server
	go s.createServiceListener()

	// Let's send any outstanding events
	go s.processEvents()

	<-s.doneChan
}
