package net

import (
	"context"
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

type LeafRpcEndpoint struct {
	sync.RWMutex
	TopoAddress  string
	VCAddress    string
	LocalAddress string

	topology *model.Topology
	vcm      *core.VCManager

	updatedServersEgExt chan *core.LeafHealthMsg
	newServersEgExt     chan *ServerAddedMessage

	vcConnPool *grpcpool.Pool
	doneChan   chan bool
}

func NewLeafRpcEndpoint(topoAddress, vcAddress, localAddress string,
	topology *model.Topology,
	vcm *core.VCManager,
	updatedServers chan *core.LeafHealthMsg,
	newServers chan *ServerAddedMessage,
) *LeafRpcEndpoint {
	return &LeafRpcEndpoint{
		TopoAddress:         topoAddress,
		VCAddress:           vcAddress,
		LocalAddress:        localAddress,
		topology:            topology,
		vcm:                 vcm,
		updatedServersEgExt: updatedServers,
		newServersEgExt:     newServers,
		doneChan:            make(chan bool, 1),
	}
}

func (s *LeafRpcEndpoint) createTopoListener() error {
	lis, err := net.Listen("tcp4", s.LocalAddress)
	if err != nil {
		logrus.Warn(err)
		return err
	}
	logrus.Infof("[LeafRPC] Creating topologyServer at %s", s.LocalAddress)
	rpcServer := grpc.NewServer()
	topoServer := NewLeafTopologyServer(s.topology, s.vcm, s.updatedServersEgExt, s.newServersEgExt)
	horus_pb.RegisterHorusTopologyServer(rpcServer, topoServer)
	return rpcServer.Serve(lis)
}

func (s *LeafRpcEndpoint) createVCConnPool() {
	s.Lock()
	defer s.Unlock()
	var factory grpcpool.Factory
	factory = func() (*grpc.ClientConn, error) {
		conn, err := grpc.Dial(s.VCAddress, grpc.WithInsecure())
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
	// 	default:
	// 		continue
	// 	}
	// }
}

func (s *LeafRpcEndpoint) Start() {
	// create a pool of gRPC connections. used to send SyncEvent messages
	s.createVCConnPool()

	// creates the end point server. used to recv SessionUpdateEvent messages
	go s.createTopoListener()

	// Let's send any outstanding events
	go s.processEvents()

	<-s.doneChan
}
