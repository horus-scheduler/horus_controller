package ctrl_sw

import (
	"time"

	"github.com/khaledmdiab/horus_controller/core"
	"github.com/khaledmdiab/horus_controller/core/net"
	"github.com/sirupsen/logrus"
)

type switchManager struct {
	status *core.CtrlStatus
	// sessionMgr *core.SessionManager

	// rpcEndPoint    *net.SwitchRpcEndpoint
	// sequencer      *sequencer.SimpleEventSequencer
	// syncJobs       chan *core.SyncJob
	// syncJobResults chan *core.SyncJobResult

	// A single end point per phy. switch to handles pkts to/from the ASIC
	asicEndPoint *asicEndPoint

	leaves []*leafController
	spines []*spineController
}

type SwitchManagerOption func(*switchManager)

func initLogger() {
	logrus.SetLevel(logrus.DebugLevel)
	logrus.SetFormatter(&logrus.TextFormatter{
		DisableLevelTruncation: true,
		FullTimestamp:          false,
		ForceColors:            true,
	})
}

func NewSwitchManager(opts ...SwitchManagerOption) *switchManager {
	// Initialize program-related structures
	initLogger()
	net.InitHorusDefinitions()

	// Read configuration
	cfg := ReadConfigFile("")

	// Initialize program status
	status := core.NewCtrlStatus()

	// activeNodeChan := make(chan *core.ActiveNodeMsg, net.DefaultUnixSockRecvSize)
	// rpcIngressChan := make(chan *horus_pb.MdcSessionUpdateEvent, net.DefaultRpcRecvSize)
	// rpcEgressChan := make(chan *horus_pb.MdcSyncEvent, net.DefaultRpcSendSize)

	// Initialize leaf and spine controllers
	var leaves []*leafController
	var spines []*spineController
	for ctrlIdx, ctrl := range cfg.Controllers {
		if ctrl.Type == "leaf" {
			newCtrl := NewLeafController(uint16(ctrlIdx+1), &ctrl, cfg)
			leaves = append(leaves, newCtrl)
		} else if ctrl.Type == "spine" {
			newCtrl := NewSpineController(uint16(ctrlIdx+1), &ctrl, cfg)
			spines = append(spines, newCtrl)
		}
	}

	// ASIC <-> CPU interface.
	asicIngress := make(chan []byte, net.DefaultUnixSockSendSize)
	asicEgress := make(chan []byte, net.DefaultUnixSockSendSize)
	asicEndPoint := NewAsicEndPoint(cfg.AsicIntf, leaves, spines, asicIngress, asicEgress)

	// rpcEndPoint := net.NewSwitchRpcEndpoint(cfg.LocalRpcAddress, cfg.RemoteRpcAddress, rpcIngressChan, rpcEgressChan)

	// syncJobs := make(chan *core.SyncJob, 1000)
	// syncJobResults := make(chan *core.SyncJobResult, 1000)
	// healthMgr := core.NewNodeHealthManager(activeNodeChan, cfg.HealthyNodeTimeOut)
	// eventSequencer := sequencer.NewSimpleEventSequencer(syncJobs, syncJobResults, healthMgr)

	// encDecChan := NewEventEncDecChan(syncJobResults, syncJobs, activeNodeChan,
	// 	rpcIngressChan, rpcEgressChan, dpRecvChan, dpSendChan)
	// evEncDec := NewEventEncDec(encDecChan, healthMgr, eventSequencer, cfg.TorId)

	s := &switchManager{
		status:       status,
		asicEndPoint: asicEndPoint,
		leaves:       leaves,
		spines:       spines,
	}

	for _, opt := range opts {
		opt(s)
	}

	s.init(cfg)

	return s
}

func (sc *switchManager) init(cfg *rootConfig) {
	// err := sc.localSock.Connect()
	// if err != nil {
	// 	log.Println("Error connecting to data path: ", err)
	// }
}

func (sc *switchManager) Run() {
	// ASIC connections
	go sc.asicEndPoint.Start()
	// go sc.rpcEndPoint.Start()

	for _, s := range sc.spines {
		go s.Start()
	}
	time.Sleep(time.Second)

	for _, l := range sc.leaves {
		go l.Start()
	}

	// Components
	// go sc.sequencer.Start()
	// go sc.healthMgr.Start()
	// go sc.evEncDec.Start()
	select {}
}
