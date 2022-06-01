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

func NewSwitchManager(topoFp, vcsFp, cfgFp string, opts ...SwitchManagerOption) *switchManager {
	// Initialize program-related structures
	initLogger()
	net.InitHorusDefinitions()

	// Read configuration
	cfg := ReadConfigFile(cfgFp)

	// Initialize program status
	status := core.NewCtrlStatus()

	// Initialize leaf and spine controllers
	var leaves []*leafController
	var spines []*spineController
	for _, ctrlID := range cfg.LeafIDs {
		newCtrl := NewLeafController(ctrlID, topoFp, vcsFp, cfg)
		leaves = append(leaves, newCtrl)
	}
	for _, ctrlID := range cfg.SpineIDs {
		newCtrl := NewSpineController(ctrlID, topoFp, vcsFp, cfg)
		spines = append(spines, newCtrl)
	}

	// ASIC <-> CPU interface.
	asicIngress := make(chan []byte, net.DefaultUnixSockSendSize)
	asicEgress := make(chan []byte, net.DefaultUnixSockSendSize)
	asicEndPoint := NewAsicEndPoint(cfg.AsicIntf, leaves, spines, asicIngress, asicEgress)

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

}

func (sc *switchManager) Run() {
	// ASIC connections
	go sc.asicEndPoint.Start()

	// Start Spine and Leaf controllers
	for _, s := range sc.spines {
		go s.Start()
	}
	time.Sleep(time.Second)

	for _, l := range sc.leaves {
		go l.Start()
	}

	select {}
}
