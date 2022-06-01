package ctrl_sw

import (
	"github.com/khaledmdiab/horus_controller/core"
	"github.com/khaledmdiab/horus_controller/core/model"
	horus_net "github.com/khaledmdiab/horus_controller/core/net"
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
	bus *LeafBus // Main leaf logic
	// rpcEndPoint *house_net.LeafRpcEndpoint // RPC server (Horus messages)
	healthMgr *core.LeafHealthManager // Tracking the health of downstream nodes
}

// Spine-specific logic
type spineController struct {
	*controller

	// Components
	bus         *SpineBus                   // Main spine logic
	rpcEndPoint *horus_net.SpineRpcEndpoint // RPC server (Horus messages)
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

	asicEgress := make(chan []byte, horus_net.DefaultUnixSockSendSize)
	asicIngress := make(chan []byte, horus_net.DefaultUnixSockRecvSize)
	hmEgress := make(chan *core.LeafHealthMsg, horus_net.DefaultUnixSockRecvSize)

	target := bfrtC.NewTarget(bfrtC.WithDeviceId(cfg.DeviceID), bfrtC.WithPipeId(cfg.PipeID))
	bfrt := bfrtC.NewClient(cfg.BfrtAddress, cfg.P4Name, uint32(ctrlID), target)

	healthMgr, err := core.NewLeafHealthManager(ctrlID, hmEgress, topology, vcm, cfg.Timeout)
	if err != nil {
		logrus.Fatal(err)
	}
	// TODO: Leaf RPC end point
	// rpcEndPoint := net.NewLeafRpcEndpoint(cfg.LocalRpcAddress, cfg.RemoteRpcAddress, rpcIngressChan, rpcEgressChan)

	ch := NewLeafBusChan(hmEgress, nil, nil, asicIngress, asicEgress)
	bus := NewLeafBus(ch, healthMgr, bfrt)
	return &leafController{
		// rpcEndPoint: nil,
		healthMgr: healthMgr,
		bus:       bus,
		controller: &controller{
			ID:          ctrlID,
			topology:    topology,
			vcm:         vcm,
			cfg:         cfg,
			bfrt:        bfrt,
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

	asicEgress := make(chan []byte, horus_net.DefaultUnixSockSendSize)
	asicIngress := make(chan []byte, horus_net.DefaultUnixSockRecvSize)
	activeNode := make(chan *core.LeafHealthMsg, horus_net.DefaultUnixSockRecvSize)
	failedLeaves := make(chan *horus_net.LeafFailedMessage, horus_net.DefaultRpcRecvSize)

	spine := topology.GetNode(ctrlID, model.NodeType_Spine)
	target := bfrtC.NewTarget(bfrtC.WithDeviceId(cfg.DeviceID), bfrtC.WithPipeId(cfg.PipeID))
	bfrt := bfrtC.NewClient(cfg.BfrtAddress, cfg.P4Name, uint32(spine.ID), target)

	rpcEndPoint := horus_net.NewSpineRpcEndpoint(spine.Address, topology, vcm, failedLeaves)
	ch := NewSpineBusChan(activeNode, failedLeaves, asicIngress, asicEgress)
	bus := NewSpineBus(ch, nil, bfrt)
	return &spineController{
		rpcEndPoint: rpcEndPoint,
		healthMgr:   nil,
		bus:         bus,
		controller: &controller{
			ID:          ctrlID,
			topology:    topology,
			vcm:         vcm,
			cfg:         cfg,
			bfrt:        bfrt,
			asicIngress: asicIngress,
			asicEgress:  asicEgress,
		},
	}
}

// Common controller init. logic goes here
func (c *controller) Start() {
}

func (c *leafController) Start() {
	logrus.
		WithFields(logrus.Fields{"ID": c.ID}).
		Infof("Starting leaf switch controller")
	c.controller.Start()
	go c.healthMgr.Start()
	go c.bus.Start()
}

func (c *spineController) Start() {
	logrus.
		WithFields(logrus.Fields{"ID": c.ID}).
		Infof("Starting spine switch controller")
	c.controller.Start()
	go c.rpcEndPoint.Start()
	go c.bus.Start()
}
