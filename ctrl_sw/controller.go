package ctrl_sw

import (
	"time"

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
	bus         *LeafBus                   // Main leaf logic
	rpcEndPoint *horus_net.LeafRpcEndpoint // RPC server (Horus messages)
	healthMgr   *core.LeafHealthManager    // Tracking the health of downstream nodes
}

// Spine-specific logic
type spineController struct {
	*controller

	// Components
	bus         *SpineBus                   // Main spine logic
	rpcEndPoint *horus_net.SpineRpcEndpoint // RPC server (Horus messages)
	healthMgr   *core.LeafHealthManager     // Tracking the health of downstream nodes
}

func NewLeafController(ctrlID uint16, topoFp string, cfg *rootConfig) *leafController {
	topoCfg := model.ReadTopologyFile(topoFp)
	topology := model.NewDCNTopology(topoCfg)
	vcm := core.NewVCManager(topology)

	asicEgress := make(chan []byte, horus_net.DefaultUnixSockSendSize)
	asicIngress := make(chan []byte, horus_net.DefaultUnixSockRecvSize)
	hmEgress := make(chan *core.LeafHealthMsg, horus_net.DefaultUnixSockRecvSize)
	updatedServersRPC := make(chan *core.LeafHealthMsg, horus_net.DefaultUnixSockRecvSize)
	newServersRPC := make(chan *horus_net.ServerAddedMessage, horus_net.DefaultUnixSockRecvSize)

	target := bfrtC.NewTarget(bfrtC.WithDeviceId(cfg.DeviceID), bfrtC.WithPipeId(cfg.PipeID))
	bfrt := bfrtC.NewClient(cfg.BfrtAddress, cfg.P4Name, uint32(ctrlID), target)

	healthMgr, err := core.NewLeafHealthManager(ctrlID, hmEgress, topology, vcm, cfg.Timeout)
	if err != nil {
		logrus.Fatal(err)
	}

	leaf, _ := topology.Leaves.Load(ctrlID)
	if leaf == nil {
		logrus.Fatal("[Leaf] Leaf %d doesn't exist in the topology", ctrlID)
	}

	rpcEndPoint := horus_net.NewLeafRpcEndpoint(cfg.TopoServer,
		cfg.VCServer,
		leaf.Address,
		topology,
		vcm,
		updatedServersRPC,
		newServersRPC)

	ch := NewLeafBusChan(hmEgress, updatedServersRPC, newServersRPC, asicIngress, asicEgress)
	bus := NewLeafBus(ch, healthMgr, topology, bfrt)
	return &leafController{
		rpcEndPoint: rpcEndPoint,
		healthMgr:   healthMgr,
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

func NewSpineController(ctrlID uint16, topoFp string, cfg *rootConfig) *spineController {
	topoCfg := model.ReadTopologyFile(topoFp)
	topology := model.NewDCNTopology(topoCfg)
	vcm := core.NewVCManager(topology)

	asicEgress := make(chan []byte, horus_net.DefaultUnixSockSendSize)
	asicIngress := make(chan []byte, horus_net.DefaultUnixSockRecvSize)
	activeNode := make(chan *core.LeafHealthMsg, horus_net.DefaultUnixSockRecvSize)
	failedLeaves := make(chan *horus_net.LeafFailedMessage, horus_net.DefaultRpcRecvSize)
	failedServers := make(chan *horus_net.ServerFailedMessage, horus_net.DefaultRpcRecvSize)
	newServers := make(chan *horus_net.ServerAddedMessage, horus_net.DefaultRpcRecvSize)

	spine := topology.GetNode(ctrlID, model.NodeType_Spine)
	target := bfrtC.NewTarget(bfrtC.WithDeviceId(cfg.DeviceID), bfrtC.WithPipeId(cfg.PipeID))
	bfrt := bfrtC.NewClient(cfg.BfrtAddress, cfg.P4Name, uint32(spine.ID), target)

	rpcEndPoint := horus_net.NewSpineRpcEndpoint(spine.Address, cfg.VCServer,
		topology, vcm, failedLeaves, failedServers, newServers)
	ch := NewSpineBusChan(activeNode, failedLeaves, failedServers, newServers, asicIngress, asicEgress)
	bus := NewSpineBus(ch, topology, nil, bfrt)
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
		Infof("[Leaf] Starting leaf switch controller")
	c.controller.Start()

	go c.rpcEndPoint.Start()
	logrus.Debugf("[Leaf] Fetching all VCs from %s", c.rpcEndPoint.VCAddress)
	time.Sleep(time.Second)
	vcs, err := c.rpcEndPoint.GetVCs()
	if len(vcs) == 0 {
		logrus.Warnf("[Leaf] No VCs were fetched from %s", c.rpcEndPoint.VCAddress)
	} else {
		logrus.Debugf("[Leaf] %d VCs were fetched", len(vcs))
	}
	if err != nil {
		logrus.Error(err)
	} else {
		for _, vcConf := range vcs {
			vc := model.NewVC(vcConf, c.topology)
			c.vcm.AddVC(vc)
		}
	}

	go c.healthMgr.Start()
	go c.bus.Start()
}

func (c *leafController) Shutdown() {
	logrus.
		WithFields(logrus.Fields{"ID": c.ID}).
		Infof("[Leaf] Shutting down leaf switch controller")
	c.bus.DoneChan <- true
	c.healthMgr.DoneChan <- true
	close(c.asicEgress)
	close(c.asicIngress)
	close(c.bus.hmMsg)
}

func (c *spineController) Start() {
	logrus.
		WithFields(logrus.Fields{"ID": c.ID}).
		Infof("[Spine] Starting spine switch controller")
	c.controller.Start()
	go c.rpcEndPoint.Start()
	logrus.Debugf("[Spine] Fetching all VCs from %s", c.rpcEndPoint.VCAddress)
	time.Sleep(time.Second)
	vcs, err := c.rpcEndPoint.GetVCs()
	if len(vcs) == 0 {
		logrus.Warnf("[Spine] No VCs were fetched from %s", c.rpcEndPoint.VCAddress)
	} else {
		logrus.Debugf("[Spine] %d VCs were fetched", len(vcs))
	}
	if err != nil {
		logrus.Error(err)
	} else {
		for _, vcConf := range vcs {
			vc := model.NewVC(vcConf, c.topology)
			c.vcm.AddVC(vc)
		}
	}

	go c.bus.Start()
}
