package ctrl_sw

import (
	"sync"
	"time"

	"github.com/khaledmdiab/horus_controller/core"
	"github.com/khaledmdiab/horus_controller/core/net"
	"github.com/sirupsen/logrus"
)

type switchManager struct {
	sync.RWMutex
	status *core.CtrlStatus

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

func NewSwitchManager(topoFp, cfgFp string, opts ...SwitchManagerOption) *switchManager {
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
		newCtrl := NewLeafController(ctrlID, topoFp, cfg)
		leaves = append(leaves, newCtrl)
	}
	for _, ctrlID := range cfg.SpineIDs {
		newCtrl := NewSpineController(ctrlID, topoFp, cfg)
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

	// Experimenting with shutting down a leaf
	/*
		go func() {
			l := sc.leaves[0]
			time.Sleep(time.Duration(10000) * time.Millisecond)
			l.Shutdown()
			logrus.Debug(">>> ", sc.asicEndPoint.leafIngress[0])
			sc.Lock()
			sc.leaves = sc.leaves[1:]
			sc.Unlock()
		}()
		for {
			time.Sleep(time.Duration(1000) * time.Millisecond)
			sc.RLock()
			logrus.Debug(len(sc.leaves))
			sc.RUnlock()
		}
	*/

	select {}
}
