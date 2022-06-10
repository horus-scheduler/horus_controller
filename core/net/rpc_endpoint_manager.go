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
	mgmtAddress string
	rpcEgress   chan *LeafFailedMessage
	doneChan    chan bool
}

func NewManagerRpcEndpoint(mgmtAddress string,
	rpcEgress chan *LeafFailedMessage,
) *ManagerRpcEndpoint {
	return &ManagerRpcEndpoint{
		mgmtAddress: mgmtAddress,
		rpcEgress:   rpcEgress,
		doneChan:    make(chan bool, 1),
	}
}

func (s *ManagerRpcEndpoint) createTopoListener() error {
	lis, err := net.Listen("tcp4", s.mgmtAddress)
	if err != nil {
		log.Println(err)
		return err
	}
	rpcServer := grpc.NewServer()
	topoServer := NewManagerTopologyServer(s.rpcEgress)
	horus_pb.RegisterHorusTopologyServer(rpcServer, topoServer)
	return rpcServer.Serve(lis)
}

// func (s *ManagerRpcEndpoint) createVCConnPool() {
// 	s.Lock()
// 	defer s.Unlock()
// 	var factory grpcpool.Factory
// 	factory = func() (*grpc.ClientConn, error) {
// 		conn, err := grpc.Dial(s.vcAddress, grpc.WithInsecure())
// 		if err != nil {
// 			logrus.Error(err)
// 		}
// 		return conn, err
// 	}
// 	var err error
// 	s.vcConnPool, err = grpcpool.New(factory, 10, 20, 5*time.Second)
// 	if err != nil {
// 		logrus.Error(err)
// 	}
// }

// func (s *LeafRpcEndpoint) GetVCs() ([]*horus_pb.VCInfo, error) {
// 	s.RLock()
// 	defer s.RUnlock()
// 	conn, err := s.vcConnPool.Get(context.Background())
// 	if err != nil {
// 		logrus.Error(err)
// 		return nil, err
// 	}
// 	defer conn.Close()
// 	client := horus_pb.NewHorusVCClient(conn.ClientConn)
// 	resp, err := client.GetVCs(context.Background(), &empty.Empty{})
// 	return resp.Vcs, err
// }

func (s *ManagerRpcEndpoint) processEvents() {
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

func (s *ManagerRpcEndpoint) Start() {
	// creates the end point server
	go s.createTopoListener()

	// Let's send any outstanding events
	go s.processEvents()

	<-s.doneChan
}
