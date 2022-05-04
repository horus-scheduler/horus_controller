package net

import (
	"context"
	"log"
	"net"
	"time"

	"github.com/processout/grpc-go-pool"
	"google.golang.org/grpc"
)

type SwitchRpcEndpoint struct {
	lAddress string
	rAddress string

	connPool   *grpcpool.Pool
	updates    chan *mdc_pb.MdcSessionUpdateEvent // from centralized controller
	syncEvents chan *mdc_pb.MdcSyncEvent          // to centralized controller
	doneChan   chan bool
}

func NewSwitchRpcEndpoint(lAddress, rAddress string,
	rpcIngressChan chan *mdc_pb.MdcSessionUpdateEvent,
	rpcEgressChan chan *mdc_pb.MdcSyncEvent) *SwitchRpcEndpoint {
	return &SwitchRpcEndpoint{
		lAddress:   lAddress,
		rAddress:   rAddress,
		updates:    rpcIngressChan,
		syncEvents: rpcEgressChan,
		doneChan:   make(chan bool, 1),
	}
}

func (s *SwitchRpcEndpoint) createListener() {
	lis, err := net.Listen("tcp4", s.lAddress)
	if err != nil {
		log.Println("11")
		log.Println(err)
		//return err
	}
	rpcServer := grpc.NewServer()
	updater := NewUpdateServer(s.updates)
	mdc_pb.RegisterMdcSessionUpdaterServer(rpcServer, updater)
	if err := rpcServer.Serve(lis); err != nil {
		log.Println("Failed to start Hello Server", err)
	}
}

func (s *SwitchRpcEndpoint) createConnPool() {
	var factory grpcpool.Factory
	factory = func() (*grpc.ClientConn, error) {
		conn, err := grpc.Dial(s.rAddress, grpc.WithInsecure())
		if err != nil {
			log.Println("HH")
			log.Println(err)
		}
		return conn, err
	}
	var err error
	s.connPool, err = grpcpool.New(factory, 10, 20, 5*time.Second)
	if err != nil {
		log.Println("YY")
		log.Println(err)
	}
}

func (s *SwitchRpcEndpoint) sendSyncEvent(e *mdc_pb.MdcSyncEvent) error {
	conn, err := s.connPool.Get(context.Background())
	if err != nil {
		log.Println(err)
		return err
	}
	defer conn.Close()
	client := mdc_pb.NewMdcControllerNotifierClient(conn.ClientConn)
	_, err = client.SyncDone(context.Background(), e)
	return err
}

func (s *SwitchRpcEndpoint) processEvents() {
	for {
		select {
		case syncEv := <-s.syncEvents:
			err := s.sendSyncEvent(syncEv)
			if err != nil {
				log.Println(err)
			}
		default:
			continue
		}
	}
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
