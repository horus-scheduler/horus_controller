package ctrl_central

import (
	"github.com/khaledmdiab/horus_controller/core"
	"github.com/khaledmdiab/horus_controller/core/label"
	"github.com/khaledmdiab/horus_controller/core/mc_algorithm"
	"github.com/khaledmdiab/horus_controller/core/model"
	"github.com/khaledmdiab/horus_controller/core/net"
	"github.com/khaledmdiab/horus_controller/core/sequencer"
	horus_pb "github.com/khaledmdiab/horus_controller/protobuf"
)

type centralController struct {
	status *core.CtrlStatus

	rpcEndPoint *net.CentralRpcEndpoint

	topology       *model.SimpleTopology
	sessionMgr     *core.SessionManager
	eventSequencer *sequencer.SimpleEventSequencer
	evEncDec       *EventEncDec
}

type CentralControllerOption func(*centralController)

func NewCentralController(opts ...CentralControllerOption) *centralController {
	status := core.NewCtrlStatus()
	cfg := ReadConfigFile("")

	topology := model.NewSimpleTopology(cfg.TorAddresses)
	algorithm := mc_algorithm.NewSimpleMCAlgorithm(topology)
	labeler := label.NewLabelCalculator(topology)

	syncJobs := make(chan *core.SyncJob, 1000)             // esEgress
	syncJobResults := make(chan *core.SyncJobResult, 1000) // esIngress
	rpcAppIngress := make(chan *horus_pb.MdcAppEvent, net.DefaultRpcRecvSize)
	rpcAppEgress := make(chan *horus_pb.MdcSyncEvent, net.DefaultRpcSendSize)
	rpcTorIngress := make(chan *horus_pb.MdcSyncEvent, net.DefaultRpcRecvSize)
	rpcTorEgress := make(chan *horus_pb.MdcSessionUpdateEvent, net.DefaultRpcSendSize)

	rpcEndPoint := net.NewCentralRpcEndpoint(cfg.AppServer, cfg.TorServer,
		topology.TorNodes, rpcAppIngress, rpcTorIngress, rpcTorEgress)

	sessionMgr := core.NewSessionManager(algorithm)
	eventSequencer := sequencer.NewSimpleEventSequencer(syncJobs, syncJobResults, nil)
	encDecChan := NewEventEncDecChan(syncJobResults, syncJobs, rpcAppIngress, rpcAppEgress, rpcTorIngress, rpcTorEgress)
	evEncDec := NewEventEncDec(topology, labeler, encDecChan, sessionMgr, eventSequencer)

	s := &centralController{
		status:      status,
		rpcEndPoint: rpcEndPoint,

		topology:       topology,
		sessionMgr:     sessionMgr,
		eventSequencer: eventSequencer,
		evEncDec:       evEncDec,
	}

	for _, opt := range opts {
		opt(s)
	}

	return s
}

func (cc *centralController) Run() {
	// RPC connections
	go cc.rpcEndPoint.Start()

	// Components
	go cc.eventSequencer.Start()
	go cc.evEncDec.Start()
	select {}
}
