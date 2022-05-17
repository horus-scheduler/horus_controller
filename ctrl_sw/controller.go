package ctrl_sw

import (
	"github.com/khaledmdiab/horus_controller/core"
	"github.com/khaledmdiab/horus_controller/core/net"
	"github.com/sirupsen/logrus"
	"google.golang.org/grpc"

	// "github.com/khaledmdiab/horus_controller/core/sequencer"
	// horus_pb "github.com/khaledmdiab/horus_controller/protobuf"
	bfrtC "github.com/khaledmdiab/bfrt-go-client/pkg/bfrt/client"
	bfrtC_pb "github.com/khaledmdiab/bfrt-go-client/pkg/bfrt/pb"
)

type controller struct {
	ID      string
	Address string
	cfg     *rootConfig
}

type leafController struct {
	*controller
}

type spineController struct {
	*controller
}

type switchCtrl struct {
	status *core.CtrlStatus
	// sessionMgr *core.SessionManager

	// rpcEndPoint    *net.SwitchRpcEndpoint
	// sequencer      *sequencer.SimpleEventSequencer
	// syncJobs       chan *core.SyncJob
	// syncJobResults chan *core.SyncJobResult

	// healthMgr  *core.NodeHealthManager
	localSock  net.LocalSock
	dpSendChan chan []byte
	dpRecvChan chan []byte
	bfrt       *bfrtC.Client

	leaves []*leafController
	spines []*spineController

	// evEncDec *EventEncDec
}

type SwitchCtrlOption func(*switchCtrl)

func NewController(opts ...SwitchCtrlOption) *switchCtrl {

	// net.InitHorusDefinitions()

	status := core.NewCtrlStatus()
	cfg := ReadConfigFile("")
	dpSendChan := make(chan []byte, net.DefaultUnixSockSendSize)
	dpRecvChan := make(chan []byte, net.DefaultUnixSockRecvSize)
	// activeNodeChan := make(chan *core.ActiveNodeMsg, net.DefaultUnixSockRecvSize)

	// rpcIngressChan := make(chan *horus_pb.MdcSessionUpdateEvent, net.DefaultRpcRecvSize)
	// rpcEgressChan := make(chan *horus_pb.MdcSyncEvent, net.DefaultRpcSendSize)

	var localSock net.LocalSock
	localSock = net.NewRawSockClient(cfg.AsicIntf, dpSendChan, dpRecvChan)

	var leaves []*leafController
	var spines []*spineController
	for _, ctrl := range cfg.Controllers {
		if ctrl.Type == "leaf" {
			leaves = append(leaves, &leafController{
				&controller{
					ID:      ctrl.ID,
					Address: ctrl.Address,
					cfg:     cfg}},
			)
		} else if ctrl.Type == "spine" {
			spines = append(spines, &spineController{
				&controller{
					ID:      ctrl.ID,
					Address: ctrl.Address,
					cfg:     cfg}},
			)
		}
	}
	// rpcEndPoint := net.NewSwitchRpcEndpoint(cfg.LocalRpcAddress, cfg.RemoteRpcAddress, rpcIngressChan, rpcEgressChan)

	// syncJobs := make(chan *core.SyncJob, 1000)
	// syncJobResults := make(chan *core.SyncJobResult, 1000)
	// healthMgr := core.NewNodeHealthManager(activeNodeChan, cfg.HealthyNodeTimeOut)
	// eventSequencer := sequencer.NewSimpleEventSequencer(syncJobs, syncJobResults, healthMgr)

	// encDecChan := NewEventEncDecChan(syncJobResults, syncJobs, activeNodeChan,
	// 	rpcIngressChan, rpcEgressChan, dpRecvChan, dpSendChan)
	// evEncDec := NewEventEncDec(encDecChan, healthMgr, eventSequencer, cfg.TorId)

	s := &switchCtrl{
		status: status,

		// // channels
		dpRecvChan: dpRecvChan,
		dpSendChan: dpSendChan,
		localSock:  localSock,

		leaves: leaves,
		spines: spines,

		// // components
		// rpcEndPoint: rpcEndPoint,
		// healthMgr:   healthMgr,
		// sequencer:   eventSequencer,
		// evEncDec:    evEncDec,
	}

	for _, opt := range opts {
		opt(s)
	}

	s.init(cfg)

	return s
}

func (sc *switchCtrl) init(cfg *rootConfig) {
	// err := sc.localSock.Connect()
	// if err != nil {
	// 	log.Println("Error connecting to data path: ", err)
	// }

	// Init BfRt client (connection pool)
	logrus.Infof("Connecting to BfRt gRPC server at %s", cfg.BfrtAddress)
	//nolint:staticcheck // SA1019
	conn, err := grpc.Dial(cfg.BfrtAddress, grpc.WithInsecure())
	if err != nil {
		logrus.Fatalf("Cannot connect to BfRt gRPC server: %v", err)
	}
	defer conn.Close()

	c := bfrtC_pb.NewBfRuntimeClient(conn)
	sc.bfrt = bfrtC.NewClient(cfg.P4Name, c, nil, "")
}

func (sc *switchCtrl) Run() {
	// // DP and RPC connections
	// go sc.localSock.Start()
	// go sc.rpcEndPoint.Start()

	// // Components
	// go sc.sequencer.Start()
	// go sc.healthMgr.Start()
	// go sc.evEncDec.Start()
	select {}
}
