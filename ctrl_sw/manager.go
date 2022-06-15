package ctrl_sw

import (
	"sync"
	"time"

	"github.com/khaledmdiab/horus_controller/core"
	horus_net "github.com/khaledmdiab/horus_controller/core/net"
	"github.com/sirupsen/logrus"
)

type switchManager struct {
	sync.RWMutex
	status *core.CtrlStatus

	// A single end point per phy. switch to handles pkts to/from the ASIC
	asicEndPoint *asicEndPoint

	rpcEndPoint *horus_net.ManagerRpcEndpoint
	rpcIngress  chan *horus_net.LeafFailedMessage

	leaves []*leafController
	spines []*spineController
}

type SwitchManagerOption func(*switchManager)

func initLogger(cfg *rootConfig) {
	if lvl, err := logrus.ParseLevel(cfg.LogLevel); err == nil {
		logrus.SetLevel(lvl)
	} else {
		logrus.SetLevel(logrus.InfoLevel)
	}
	logrus.SetFormatter(&logrus.TextFormatter{
		DisableLevelTruncation: true,
		FullTimestamp:          false,
		ForceColors:            true,
	})
}

func NewSwitchManager(topoFp, cfgFp string, opts ...SwitchManagerOption) *switchManager {
	logrus.SetLevel(logrus.TraceLevel)
	// Read configuration
	cfg := ReadConfigFile(cfgFp)

	// Initialize program-related structures
	initLogger(cfg)
	horus_net.InitHorusDefinitions()

	// Initialize program status
	status := core.NewCtrlStatus()

	// Initialize leaf and spine controllers
	var leaves []*leafController
	var spines []*spineController
	for _, ctrlID := range cfg.LeafIDs {
		newCtrl := NewLeafController(ctrlID, topoFp, cfg)
		leaves = append(leaves, newCtrl)
	}
	for _, ctrlID := range cfg.SpineIDs {
		newCtrl := NewSpineController(ctrlID, topoFp, cfg)
		spines = append(spines, newCtrl)
	}

	// ASIC <-> CPU interface.
	asicIngress := make(chan []byte, horus_net.DefaultUnixSockSendSize)
	asicEgress := make(chan []byte, horus_net.DefaultUnixSockSendSize)
	rpcIngress := make(chan *horus_net.LeafFailedMessage, horus_net.DefaultUnixSockSendSize)
	asicEndPoint := NewAsicEndPoint(cfg.AsicIntf, leaves, spines, asicIngress, asicEgress)
	rpcEndPoint := horus_net.NewManagerRpcEndpoint(cfg.MgmtAddress, rpcIngress)
	s := &switchManager{
		status:       status,
		asicEndPoint: asicEndPoint,
		rpcEndPoint:  rpcEndPoint,
		rpcIngress:   rpcIngress,
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
	go sc.rpcEndPoint.Start()
	time.Sleep(time.Second)

	// Start Spine and Leaf controllers
	for _, s := range sc.spines {
		go s.Start()
	}
	time.Sleep(time.Second)

	for _, l := range sc.leaves {
		go l.Start()
	}

	for {
		select {
		case failed := <-sc.rpcIngress:
			logrus.Debugf("[Manager] Removing leaf %d", failed.Leaf.Id)
			var removedLeaf *leafController = nil
			removedLeafIdx := -1
			for leafIdx, leaf := range sc.leaves {
				if leaf.ID == uint16(failed.Leaf.Id) {
					removedLeaf = leaf
					removedLeafIdx = leafIdx
				}
			}
			if removedLeafIdx > -1 {
				removedLeaf.Shutdown()
				time.Sleep(time.Second)
				sc.Lock()
				logrus.Debugf("[Manager] Leaves count = %d", len(sc.leaves))
				sc.leaves = append(sc.leaves[:removedLeafIdx], sc.leaves[removedLeafIdx+1:]...)
				logrus.Debugf("[Manager] Leaves count = %d", len(sc.leaves))
				sc.Unlock()
			}
		default:
			continue
		}
	}
}
