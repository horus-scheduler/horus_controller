package ctrl_switch

import (
	"log"

	"github.com/khaledmdiab/horus_controller/core"
	"github.com/khaledmdiab/horus_controller/core/net"
	horus_pb "github.com/khaledmdiab/horus_controller/protobuf"
	"github.com/khaledmdiab/horus_controller/sequencer"
)

type mdcSwitchCtrl struct {
	status     *core.CtrlStatus
	sessionMgr *core.SessionManager

	rpcEndPoint    *net.SwitchRpcEndpoint
	sequencer      *sequencer.SimpleEventSequencer
	syncJobs       chan *core.SyncJob
	syncJobResults chan *core.SyncJobResult

	healthMgr  *core.NodeHealthManager
	localSock  net.LocalSock
	dpSendChan chan []byte
	dpRecvChan chan []byte

	evEncDec *EventEncDec
}

type SwitchCtrlOption func(*mdcSwitchCtrl)

func NewController(opts ...SwitchCtrlOption) *mdcSwitchCtrl {

	net.InitMdcDefinitions()

	status := core.NewCtrlStatus()
	cfg := ReadConfigFile("")

	dpSendChan := make(chan []byte, net.DefaultUnixSockSendSize)
	dpRecvChan := make(chan []byte, net.DefaultUnixSockRecvSize)
	activeNodeChan := make(chan *core.ActiveNodeMsg, net.DefaultUnixSockRecvSize)

	rpcIngressChan := make(chan *horus_pb.MdcSessionUpdateEvent, net.DefaultRpcRecvSize)
	rpcEgressChan := make(chan *horus_pb.MdcSyncEvent, net.DefaultRpcSendSize)

	var localSock net.LocalSock
	if cfg.LocalSockType == "unix" {
		localSock = net.NewUnixSockClient(cfg.LocalSockPath, dpSendChan, dpRecvChan)
	} else if cfg.LocalSockType == "raw" {
		localSock = net.NewRawSockClient(cfg.LocalSockPath, dpSendChan, dpRecvChan)
	}

	rpcEndPoint := net.NewSwitchRpcEndpoint(cfg.LocalRpcAddress, cfg.RemoteRpcAddress, rpcIngressChan, rpcEgressChan)

	syncJobs := make(chan *core.SyncJob, 1000)
	syncJobResults := make(chan *core.SyncJobResult, 1000)
	healthMgr := core.NewNodeHealthManager(activeNodeChan, cfg.HealthyNodeTimeOut)
	eventSequencer := sequencer.NewSimpleEventSequencer(syncJobs, syncJobResults, healthMgr)

	encDecChan := NewEventEncDecChan(syncJobResults, syncJobs, activeNodeChan,
		rpcIngressChan, rpcEgressChan, dpRecvChan, dpSendChan)
	evEncDec := NewEventEncDec(encDecChan, healthMgr, eventSequencer, cfg.TorId)

	s := &mdcSwitchCtrl{
		status: status,

		// channels
		dpRecvChan: dpRecvChan,
		dpSendChan: dpSendChan,
		//syncJobs:       syncJobs,
		//syncJobResults: syncJobResults,
		localSock: localSock,

		// components
		rpcEndPoint: rpcEndPoint,
		healthMgr:   healthMgr,
		sequencer:   eventSequencer,
		evEncDec:    evEncDec,
	}

	for _, opt := range opts {
		opt(s)
	}

	s.init()

	return s
}

func (sc *mdcSwitchCtrl) init() {
	err := sc.localSock.Connect()
	if err != nil {
		log.Println("Error connecting to data path: ", err)
	}
}

func (sc *mdcSwitchCtrl) Run() {
	// DP and RPC connections
	go sc.localSock.Start()
	//time.Sleep(1 * time.Second)
	go sc.rpcEndPoint.Start()
	//time.Sleep(1 * time.Second)

	// Components
	go sc.sequencer.Start()
	//time.Sleep(1 * time.Second)
	go sc.healthMgr.Start()
	//time.Sleep(1 * time.Second)
	go sc.evEncDec.Start()
	select {}
}
