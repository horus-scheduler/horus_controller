package ctrl_sw

import (
	"github.com/khaledmdiab/horus_controller/core"
	"github.com/khaledmdiab/horus_controller/core/net"
	"github.com/sirupsen/logrus"

	bfrtC "github.com/khaledmdiab/bfrt-go-client/pkg/client"
)

type controller struct {
	ID      uint16
	Address string
	cfg     *rootConfig

	// RPC server
	rpcEndPoint *net.SwitchRpcEndpoint

	// This controller tracks the health of its downstream nodes
	healthMgr *core.NodeHealthManager

	// recv-from the ASIC
	asicIngress chan []byte
	// send-to the ASIC
	asicEgress chan []byte
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
	asicEndPoint *asicEndPoint
	bfrt         *bfrtC.Client

	leaves []*leafController
	spines []*spineController

	// evEncDec *EventEncDec
}

type SwitchCtrlOption func(*switchCtrl)

func initLogger() {
	logrus.SetLevel(logrus.DebugLevel)

	logrus.SetFormatter(&logrus.TextFormatter{
		DisableLevelTruncation: true,
		FullTimestamp:          false,
		ForceColors:            true,
	})
}

func NewController(opts ...SwitchCtrlOption) *switchCtrl {

	initLogger()
	// net.InitHorusDefinitions()

	status := core.NewCtrlStatus()
	cfg := ReadConfigFile("")
	// activeNodeChan := make(chan *core.ActiveNodeMsg, net.DefaultUnixSockRecvSize)

	// rpcIngressChan := make(chan *horus_pb.MdcSessionUpdateEvent, net.DefaultRpcRecvSize)
	// rpcEgressChan := make(chan *horus_pb.MdcSyncEvent, net.DefaultRpcSendSize)

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
	// ASIC <-> CPU interface.
	asicEndPoint := NewAsicEndPoint(cfg.AsicIntf, leaves, spines)
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
		asicEndPoint: asicEndPoint,

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
	device := bfrtC.NewTarget(bfrtC.WithDeviceId(cfg.DeviceID), bfrtC.WithPipeId(cfg.PipeID))
	sc.bfrt = bfrtC.NewClient(cfg.BfrtAddress, cfg.P4Name, device)
}

func (sc *switchCtrl) Run() {
	// // DP and RPC connections
	go sc.asicEndPoint.Start()
	// go sc.rpcEndPoint.Start()

	// // Components
	// go sc.sequencer.Start()
	// go sc.healthMgr.Start()
	// go sc.evEncDec.Start()
	select {}
}
