package net

import (
	"context"
	"log"
	"time"

	horus_pb "github.com/khaledmdiab/horus_controller/protobuf"
	grpcpool "github.com/processout/grpc-go-pool"
	"google.golang.org/grpc"
)

type SwitchRpcEndpoint struct {
	lAddress string
	rAddress string

	connPool *grpcpool.Pool
	incoming chan *horus_pb.HorusMessage // incoming messages
	outgoing chan *horus_pb.HorusMessage // outgoing messages
	doneChan chan bool
}

func NewSwitchRpcEndpoint(lAddress, rAddress string,
	rpcIngressChan chan *horus_pb.HorusMessage,
	rpcEgressChan chan *horus_pb.HorusMessage) *SwitchRpcEndpoint {
	return &SwitchRpcEndpoint{
		lAddress: lAddress,
		rAddress: rAddress,
		incoming: rpcIngressChan,
		outgoing: rpcEgressChan,
		doneChan: make(chan bool, 1),
	}
}

func (s *SwitchRpcEndpoint) createListener() {
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

func (s *SwitchRpcEndpoint) createConnPool() {
	var factory grpcpool.Factory
	factory = func() (*grpc.ClientConn, error) {
		conn, err := grpc.Dial(s.rAddress, grpc.WithInsecure())
		if err != nil {
			log.Println(err)
		}
		return conn, err
	}
	var err error
	s.connPool, err = grpcpool.New(factory, 10, 20, 5*time.Second)
	if err != nil {
		log.Println(err)
	}
}

func (s *SwitchRpcEndpoint) sendSyncEvent(e *horus_pb.MdcSyncEvent) error {
	conn, err := s.connPool.Get(context.Background())
	if err != nil {
		log.Println(err)
		return err
	}
	defer conn.Close()
	client := horus_pb.NewMdcControllerNotifierClient(conn.ClientConn)
	_, err = client.SyncDone(context.Background(), e)
	return err
}

func (s *SwitchRpcEndpoint) processEvents() {
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

func (s *SwitchRpcEndpoint) Start() {
	// create a pool of gRPC connections. used to send SyncEvent messages
	s.createConnPool()

	// creates the end point server. used to recv SessionUpdateEvent messages
	go s.createListener()

	// Let's send any outstanding events
	go s.processEvents()
	<-s.doneChan
}
