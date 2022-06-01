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

func initLogger() {
	logrus.SetLevel(logrus.DebugLevel)
	logrus.SetFormatter(&logrus.TextFormatter{
		DisableLevelTruncation: true,
		FullTimestamp:          false,
		ForceColors:            true,
	})
}

func NewCentralController(opts ...CentralControllerOption) *centralController {
	initLogger()

	status := core.NewCtrlStatus()
	binCfg := model.ReadConfigFile("")
	topoCfg := model.ReadTopologyFile("")
	vcsConf := model.ReadVCsFile("")
	// logrus.Info(binCfg)
	// logrus.Info(topoCfg)
	// logrus.Info(vcsConf)

	topology := model.NewDCNTopology(topoCfg)
	vcm := core.NewVCManager(topology)
	for _, vcConf := range vcsConf.VCs {
		vc := model.NewVC(vcConf, topology)
		vcm.AddVC(vc)
	}

	rpcEndPoint := net.NewCentralRpcEndpoint(binCfg.TopoServer, topology, vcm)
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
