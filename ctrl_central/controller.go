package ctrl_central

import (
	"github.com/horus-scheduler/horus_controller/core"
	"github.com/horus-scheduler/horus_controller/core/model"
	"github.com/horus-scheduler/horus_controller/core/net"
	horus_net "github.com/horus-scheduler/horus_controller/core/net"
	"github.com/sirupsen/logrus"
)

type centralController struct {
	status      *core.CtrlStatus
	rpcEndPoint *net.CentralRpcEndpoint
	bus         *CentralBus
	topology    *model.Topology
	vcm         *core.VCManager
	cps         []CentralizedCP
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
	topology := model.NewDCNFromConf(topoCfg)
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

	enabledPorts := make(chan *horus_net.PortEnabledMessage, horus_net.DefaultRpcRecvSize)
	disabledPorts := make(chan *horus_net.PortDisabledMessage, horus_net.DefaultRpcRecvSize)
	rpcEndPoint := net.NewCentralRpcEndpoint(
		binCfg.SrvServer,
		topology,
		vcm,
		enabledPorts,
		disabledPorts,
	)

	var cps []CentralizedCP
	for _, asicCfg := range topoCfg.AsicConf.Asics {
		var cp CentralizedCP
		switch asicCfg.CtrlAPI {
		case "fake":
			cp = NewFakeCentralizedCP(asicCfg.ID, topology)
		case "bfrt":
			cp = NewBfrtCentralizedCP(asicCfg.ID, topology)
		default:
			logrus.Fatalf("[Centralized] Control API %s is invalid!", asicCfg.CtrlAPI)
		}
		cps = append(cps, cp)
	}

	bus := NewCentralBus(topology, vcm, cps,
		NewCentralBusChan(enabledPorts, disabledPorts))

	s := &centralController{
		status:      status,
		rpcEndPoint: rpcEndPoint,
		topology:    topology,
		vcm:         vcm,
		bus:         bus,
		cps:         cps,
	}

	for _, opt := range opts {
		opt(s)
	}

	return s
}

func (cc *centralController) Run() {
	// RPC connections
	go cc.rpcEndPoint.Start()

	for _, cp := range cc.cps {
		cp.InitPorts()
	}

	// Components
	go cc.bus.Start()
	select {}
}
