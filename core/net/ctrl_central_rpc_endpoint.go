package net

import (
	"context"
	"github.com/khaledmdiab/horus_controller/core/model"
	"github.com/khaledmdiab/horus_controller/protobuf"
	"errors"
	"github.com/processout/grpc-go-pool"
	"google.golang.org/grpc"
	"log"
	"net"
	"time"
)

// Two Rpc Endpoints: App <-> Controller and ToR <-> Controller
type CentralRpcEndpoint struct {
	appLAddr string
	torLAddr string

	appIngressEvents chan *mdc_pb.MdcAppEvent
	torIngressEvents chan *mdc_pb.MdcSyncEvent
	torEgressEvents  chan *mdc_pb.MdcSessionUpdateEvent // to ToR

	torNodes    []*model.Node
	torConnPool []*grpcpool.Pool

	doneChan chan bool
}

func NewCentralRpcEndpoint(appLAddr, torLAddr string, torNodes []*model.Node,
	appIngressEvents chan *mdc_pb.MdcAppEvent,
	torIngressEvents chan *mdc_pb.MdcSyncEvent,
	torEgressEvents chan *mdc_pb.MdcSessionUpdateEvent,
) *CentralRpcEndpoint {
	torCount := len(torNodes)
	connPool := make([]*grpcpool.Pool, torCount)
	return &CentralRpcEndpoint{
		appLAddr:         appLAddr,
		torLAddr:         torLAddr,
		appIngressEvents: appIngressEvents,
		torIngressEvents: torIngressEvents,
		torEgressEvents:  torEgressEvents,
		torNodes:         torNodes,
		torConnPool:      connPool,
		doneChan:         make(chan bool, 1),
	}
}

func (s *CentralRpcEndpoint) createAppListener() error {
	lis, err := net.Listen("tcp4", s.appLAddr)
	if err != nil {
		log.Println(err)
		return err
	}
	rpcServer := grpc.NewServer()
	appServer := NewAppServer(s.appIngressEvents)
	mdc_pb.RegisterMdcAppNotifierServer(rpcServer, appServer)
	return rpcServer.Serve(lis)
}

func (s *CentralRpcEndpoint) createTorListener() error {
	lis, err := net.Listen("tcp4", s.torLAddr)
	if err != nil {
		log.Println(err)
		return err
	}
	rpcServer := grpc.NewServer()
	torServer := NewTorServer(s.torIngressEvents)
	mdc_pb.RegisterMdcControllerNotifierServer(rpcServer, torServer)
	return rpcServer.Serve(lis)
}

func (s *CentralRpcEndpoint) createTorConnPool(torAddr string) *grpcpool.Pool {
	var factory grpcpool.Factory
	factory = func() (*grpc.ClientConn, error) {
		conn, err := grpc.Dial(torAddr, grpc.WithInsecure())
		if err != nil {
			log.Println(err)
		}
		return conn, err
	}
	var err error
	connPool, err := grpcpool.New(factory, 2, 6, 5*time.Second)
	if err != nil {
		log.Println(err)
	}
	return connPool
}

func (s *CentralRpcEndpoint) sendUpdateStateEvent(e *mdc_pb.MdcSessionUpdateEvent) error {
	if e.TorId < 0 || e.TorId >= uint32(len(s.torConnPool)) {
		return errors.New("invalid tor id")
	}
	conn, err := s.torConnPool[e.TorId].Get(context.Background())
	if err != nil {
		log.Println(err)
		return err
	}
	defer conn.Close()

	client := mdc_pb.NewMdcSessionUpdaterClient(conn.ClientConn)
	_, err = client.UpdateState(context.Background(), e)
	return err
}

func (s *CentralRpcEndpoint) processEvents() {
	for {
		select {
		case updateStateEv := <-s.torEgressEvents:
			err := s.sendUpdateStateEvent(updateStateEv)
			if err != nil {
				log.Println(err)
			}
		default:
			continue
		}
	}
}

func (s *CentralRpcEndpoint) Start() {
	// create an array of connection pools of gRPC clients.
	// each gRPC client corresponds to a ToR controller.
	torCount := len(s.torNodes)
	for torId := 0; torId < torCount; torId += 1 {
		connPool := s.createTorConnPool(s.torNodes[torId].Address)
		s.torConnPool[torId] = connPool
	}

	// creates the end point servers for both App and ToR
	go s.createAppListener()
	go s.createTorListener()

	// Let's send any outstanding events
	go s.processEvents()
	<-s.doneChan
}
