package ctrl_central

import (
	"github.com/khaledmdiab/horus_controller/core"
	"github.com/khaledmdiab/horus_controller/core/model"
	"github.com/khaledmdiab/horus_controller/core/net"
	"github.com/sirupsen/logrus"
)

type centralController struct {
	status      *core.CtrlStatus
	rpcEndPoint *net.CentralRpcEndpoint
	bus         *CentralBus
	topology    *model.Topology
	vcm         *core.VCManager
}

type CentralControllerOption func(*centralController)

func initLogger(cfg *model.BinRootConfig) {
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

func NewCentralController(opts ...CentralControllerOption) *centralController {
	logrus.SetLevel(logrus.TraceLevel)
	status := core.NewCtrlStatus()
	binCfg := model.ReadConfigFile("")
	topoCfg := model.ReadTopologyFile("")
	vcsConf := model.ReadVCsFile("")

	initLogger(binCfg)

	logrus.Info("[Central] Initializing Topology and VC Manager...")
	topology := model.NewDCNTopology(topoCfg)
	vcm := core.NewVCManager(topology)
	for _, vcConf := range vcsConf.VCs {
		vc, err := model.NewVC(vcConf, topology)
		if err != nil {
			logrus.Error(err)
		} else {
			vcm.AddVC(vc)
		}
	}

	logrus.Info("[Central] Topology and VC Manager are initialized")

	rpcEndPoint := net.NewCentralRpcEndpoint(binCfg.SrvServer, topology, vcm)
	bus := NewCentralBus(topology, vcm, NewCentralBusChan())

	s := &centralController{
		status:      status,
		rpcEndPoint: rpcEndPoint,
		topology:    topology,
		vcm:         vcm,
		bus:         bus,
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
	// go cc.eventSequencer.Start()
	go cc.bus.Start()
	select {}
}
