package ctrl_sw

import (
	"github.com/khaledmdiab/horus_controller/core"
	"github.com/khaledmdiab/horus_controller/core/model"
	house_net "github.com/khaledmdiab/horus_controller/core/net"
	"github.com/sirupsen/logrus"

	bfrtC "github.com/khaledmdiab/bfrt-go-client/pkg/client"
)

type controller struct {
	ID      uint16
	Address string
	cfg     *rootConfig

	topology *model.Topology
	vcm      *core.VCManager

	// Controller components
	bfrt *bfrtC.Client // BfRt client

	// Communicating with the ASIC
	asicIngress chan []byte // recv-from the ASIC
	asicEgress  chan []byte // send-to the ASIC
}

// Leaf-specific logic
type leafController struct {
	*controller

	// Components
	bus         *LeafBus                   // Main leaf logic
	rpcEndPoint *house_net.LeafRpcEndpoint // RPC server (Horus messages)
	healthMgr   *core.LeafHealthManager    // Tracking the health of downstream nodes
}

// Spine-specific logic
type spineController struct {
	*controller

	// Components
	bus         *SpineBus                   // Main spine logic
	rpcEndPoint *house_net.SpineRpcEndpoint // RPC server (Horus messages)
	healthMgr   *core.LeafHealthManager     // Tracking the health of downstream nodes
}

func NewLeafController(ctrlID uint16, topoFp, vcsFp string, cfg *rootConfig) *leafController {
	topoCfg := model.ReadTopologyFile(topoFp)
	vcsConf := model.ReadVCsFile(vcsFp)
	topology := model.NewDCNTopology(topoCfg)
	vcm := core.NewVCManager(topology)
	for _, vcConf := range vcsConf.VCs {
		vc := model.NewVC(vcConf, topology)
		vcm.AddVC(vc)
	}

	asicEgress := make(chan []byte, house_net.DefaultUnixSockSendSize)
	asicIngress := make(chan []byte, house_net.DefaultUnixSockRecvSize)
	hmEgress := make(chan *core.LeafHealthMsg, house_net.DefaultUnixSockRecvSize)
	// rpcIngress := make(chan *horus_pb.HorusMessage, house_net.DefaultRpcRecvSize)
	// rpcEgress := make(chan *horus_pb.HorusMessage, house_net.DefaultRpcSendSize)

	healthMgr, err := core.NewLeafHealthManager(ctrlID, hmEgress, topology, vcm, cfg.Timeout)
	if err != nil {
		logrus.Fatal(err)
	}
	// TODO: Leaf RPC end point
	// rpcEndPoint := net.NewLeafRpcEndpoint(cfg.LocalRpcAddress, cfg.RemoteRpcAddress, rpcIngressChan, rpcEgressChan)
	// TODO: object model

	ch := NewLeafBusChan(hmEgress, nil, nil, asicIngress, asicEgress)

	bus := NewLeafBus(ch, healthMgr)
	return &leafController{
		rpcEndPoint: nil,
		healthMgr:   healthMgr,
		bus:         bus,
		controller: &controller{
			ID:          ctrlID,
			topology:    topology,
			vcm:         vcm,
			cfg:         cfg,
			asicIngress: asicIngress,
			asicEgress:  asicEgress,
		},
	}
}

func NewSpineController(ctrlID uint16, topoFp, vcsFp string, cfg *rootConfig) *spineController {
	topoCfg := model.ReadTopologyFile(topoFp)
	vcsConf := model.ReadVCsFile(vcsFp)
	topology := model.NewDCNTopology(topoCfg)
	vcm := core.NewVCManager(topology)
	for _, vcConf := range vcsConf.VCs {
		vc := model.NewVC(vcConf, topology)
		vcm.AddVC(vc)
	}

	asicEgress := make(chan []byte, house_net.DefaultUnixSockSendSize)
	asicIngress := make(chan []byte, house_net.DefaultUnixSockRecvSize)
	activeNode := make(chan *core.LeafHealthMsg, house_net.DefaultUnixSockRecvSize)
	// rpcIngress := make(chan *horus_pb.HorusMessage, house_net.DefaultRpcRecvSize)
	// rpcEgress := make(chan *horus_pb.HorusMessage, house_net.DefaultRpcSendSize)

	// TODO: Spine Health manager
	// healthMgr := nil //core.NewLeafHealthManager(activeNode, 1000)
	// TODO: Spine RPC end point
	// rpcEndPoint := net.NewSpineRpcEndpoint(cfg.LocalRpcAddress, cfg.RemoteRpcAddress, rpcIngressChan, rpcEgressChan)
	// TODO: object model

	ch := NewSpineBusChan(activeNode, nil, nil, asicIngress, asicEgress)

	bus := NewSpineBus(ch, nil)
	return &spineController{
		rpcEndPoint: nil,
		healthMgr:   nil,
		bus:         bus,
		controller: &controller{
			ID:          ctrlID,
			topology:    topology,
			vcm:         vcm,
			cfg:         cfg,
			asicIngress: asicIngress,
			asicEgress:  asicEgress,
		},
	}
}

// Common controller init. logic goes here
func (c *controller) Start() {
	logrus.
		WithFields(logrus.Fields{"ID": c.ID}).
		Infof("Starting the switch controller")
	target := bfrtC.NewTarget(bfrtC.WithDeviceId(c.cfg.DeviceID), bfrtC.WithPipeId(c.cfg.PipeID))
	c.bfrt = bfrtC.NewClient(c.cfg.BfrtAddress, c.cfg.P4Name, uint32(c.ID), target)
}

func (c *leafController) Start() {
	c.controller.Start()
	go c.healthMgr.Start()
	go c.bus.Start()
}

func (c *spineController) Start() {
	c.controller.Start()
	// go c.healthMgr.Start()
	// go c.evEncDec.Start()
}
