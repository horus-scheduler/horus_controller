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
	SrvCentralAddr string
	SrvLAddr       string

	topology *model.Topology
	vcm      *core.VCManager

	updatedServersEgExt chan *core.LeafHealthMsg
	newServersEgExt     chan *ServerAddedMessage
	newVCsEgExt         chan *VCUpdatedMessage

	centralConnPool *grpcpool.Pool
	doneChan        chan bool
}

func NewLeafRpcEndpoint(srvLAddr, srvCentralAddr string,
	topology *model.Topology,
	vcm *core.VCManager,
	updatedServers chan *core.LeafHealthMsg,
	newServers chan *ServerAddedMessage,
	newVCs chan *VCUpdatedMessage,
) *LeafRpcEndpoint {
	return &LeafRpcEndpoint{
		SrvLAddr:            srvLAddr,
		SrvCentralAddr:      srvCentralAddr,
		topology:            topology,
		vcm:                 vcm,
		updatedServersEgExt: updatedServers,
		newServersEgExt:     newServers,
		newVCsEgExt:         newVCs,
		doneChan:            make(chan bool, 1),
	}
}

func (s *LeafRpcEndpoint) createServiceListener() error {
	lis, err := net.Listen("tcp4", s.SrvLAddr)
	if err != nil {
		logrus.Warn(err)
		return err
	}
	logrus.Infof("[LeafRPC] Creating topologyServer at %s", s.SrvLAddr)
	rpcServer := grpc.NewServer()
	topoServer := NewLeafSrvServer(s.topology, s.vcm,
		s.updatedServersEgExt,
		s.newServersEgExt, s.newVCsEgExt)
	horus_pb.RegisterHorusServiceServer(rpcServer, topoServer)
	return rpcServer.Serve(lis)
}

func (s *LeafRpcEndpoint) createConnPool() {
	s.Lock()
	defer s.Unlock()
	var factory grpcpool.Factory
	factory = func() (*grpc.ClientConn, error) {
		conn, err := grpc.Dial(s.SrvCentralAddr, grpc.WithInsecure())
		if err != nil {
			logrus.Error(err)
		}
		return conn, err
	}
	var err error
	s.centralConnPool, err = grpcpool.New(factory, 10, 20, 5*time.Second)
	if err != nil {
		logrus.Error(err)
	}
}

func (s *LeafRpcEndpoint) GetVCs() ([]*horus_pb.VCInfo, error) {
	s.RLock()
	defer s.RUnlock()
	conn, err := s.centralConnPool.Get(context.Background())
	if err != nil {
		logrus.Error(err)
		return nil, err
	}
	defer conn.Close()
	client := horus_pb.NewHorusServiceClient(conn.ClientConn)
	resp, err := client.GetVCs(context.Background(), &empty.Empty{})
	return resp.Vcs, err
}

func (s *LeafRpcEndpoint) processEvents() {
}

func (s *LeafRpcEndpoint) Start() {
	// create a pool of gRPC connections. used to send SyncEvent messages
	s.createConnPool()

	// creates the end point server. used to recv SessionUpdateEvent messages
	go s.createServiceListener()

	// Let's send any outstanding events
	go s.processEvents()

	<-s.doneChan
}
