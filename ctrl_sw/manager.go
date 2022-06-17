package ctrl_sw

import (
	"sync"
	"time"

	"github.com/khaledmdiab/horus_controller/core"
	"github.com/khaledmdiab/horus_controller/core/model"
	horus_net "github.com/khaledmdiab/horus_controller/core/net"
	"github.com/sirupsen/logrus"
)

type switchManager struct {
	sync.RWMutex
	topoFp string
	cfg    *rootConfig
	status *core.CtrlStatus

	// A single end point per phy. switch to handles pkts to/from the ASIC
	asicEndPoint *asicEndPoint

	rpcEndPoint  *horus_net.ManagerRpcEndpoint
	failedLeaves chan *horus_net.LeafFailedMessage
	newLeaves    chan *horus_net.LeafAddedMessage

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
	failedLeaves := make(chan *horus_net.LeafFailedMessage, horus_net.DefaultUnixSockSendSize)
	newLeaves := make(chan *horus_net.LeafAddedMessage, horus_net.DefaultUnixSockSendSize)
	asicEndPoint := NewAsicEndPoint(cfg.AsicIntf, leaves, spines, asicIngress, asicEgress)
	rpcEndPoint := horus_net.NewManagerRpcEndpoint(cfg.MgmtAddress, failedLeaves, newLeaves)
	s := &switchManager{
		topoFp:       topoFp,
		cfg:          cfg,
		status:       status,
		asicEndPoint: asicEndPoint,
		rpcEndPoint:  rpcEndPoint,
		failedLeaves: failedLeaves,
		newLeaves:    newLeaves,
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
		case newLeaf := <-sc.newLeaves:
			leafID := uint16(newLeaf.Leaf.Id)
			logrus.Debugf("[Manager] Adding leaf %d", leafID)
			sc.Lock()
			logrus.Debugf("[Manager] Leaves count = %d", len(sc.leaves))
			leafCtrl := NewLeafController(leafID, sc.topoFp, sc.cfg, WithExtraLeaf(newLeaf.Leaf))
			leaf := leafCtrl.topology.GetNode(leafID, model.NodeType_Leaf)
			if leaf != nil {
				sc.leaves = append(sc.leaves, leafCtrl)
				sc.asicEndPoint.AddLeafCtrl(leafCtrl)
				go leafCtrl.Start()
			} else {
				logrus.Errorf("[Manager] Leaf %d was not added", leafID)
			}
			logrus.Debugf("[Manager] Leaves count = %d", len(sc.leaves))
			sc.Unlock()

		case failedLeaf := <-sc.failedLeaves:
			logrus.Debugf("[Manager] Removing leaf %d", failedLeaf.Leaf.Id)
			var removedLeaf *leafController = nil
			removedLeafIdx := -1
			for leafIdx, leaf := range sc.leaves {
				if leaf.ID == uint16(failedLeaf.Leaf.Id) {
					removedLeaf = leaf
					removedLeafIdx = leafIdx
				}
			}
			if removedLeafIdx > -1 {
				removedLeaf.Shutdown()
				time.Sleep(time.Second)
				sc.Lock()
				logrus.Debugf("[Manager] Leaves count = %d", len(sc.leaves))
				sc.asicEndPoint.RemoveLeafCtrl(removedLeaf)
				sc.leaves = append(sc.leaves[:removedLeafIdx], sc.leaves[removedLeafIdx+1:]...)
				logrus.Debugf("[Manager] Leaves count = %d", len(sc.leaves))
				sc.Unlock()
			}
		default:
			continue
		}
	}
}
