package ctrl_mgr

import (
	"context"
	"fmt"
	"time"

	"github.com/horus-scheduler/horus_controller/core"
	"github.com/horus-scheduler/horus_controller/core/model"
	horus_net "github.com/horus-scheduler/horus_controller/core/net"
	horus_pb "github.com/horus-scheduler/horus_controller/protobuf"
	bfrtC "github.com/khaledmdiab/bfrt-go-client/pkg/client"
	"github.com/khaledmdiab/bfrt-go-client/pkg/pb"
	"github.com/sirupsen/logrus"
)

type controller struct {
	ID      uint16
	Address string
	cfg     *rootConfig

	topology *model.Topology
	vcm      *core.VCManager

	// Controller components
	pipeID uint32

	// Communicating with the ASIC
	asicIngress chan []byte // recv-from the ASIC
	asicEgress  chan []byte // send-to the ASIC
}

// Leaf-specific logic
type leafController struct {
	*controller

	cp LeafCP

	// Components
	ch          *LeafBusChan
	bus         *LeafBus                   // Main leaf logic
	rpcEndPoint *horus_net.LeafRpcEndpoint // RPC server (Horus messages)
	healthMgr   *core.LeafHealthManager    // Tracking the health of downstream nodes
}

// Spine-specific logic
type spineController struct {
	*controller

	cp SpineCP

	// Components
	bus         *SpineBus                   // Main spine logic
	rpcEndPoint *horus_net.SpineRpcEndpoint // RPC server (Horus messages)
	healthMgr   *core.LeafHealthManager     // Tracking the health of downstream nodes
}

type TopologyOption func(*model.Topology) error
type BfrtUpdateFn func(context.Context, *pb.TableEntry) error

func WithExtraLeaf(leafInfo *horus_pb.LeafInfo) TopologyOption {
	return func(topo *model.Topology) error {
		_, err := topo.AddLeafToSpine(leafInfo)
		return err
	}
}

func WithoutLeaves() TopologyOption {
	return func(topo *model.Topology) error {
		topo.ClearLeaves()
		return nil
	}
}

func NewBareLeafController(ctrlID uint16, pipeID uint32, cfg *rootConfig, opts ...TopologyOption) *leafController {
	asicEgress := make(chan []byte, horus_net.DefaultUnixSockSendSize)
	asicIngress := make(chan []byte, horus_net.DefaultUnixSockRecvSize)
	hmEgress := make(chan *core.LeafHealthMsg, horus_net.DefaultUnixSockRecvSize)
	updatedServersRPC := make(chan *core.LeafHealthMsg, horus_net.DefaultUnixSockRecvSize)
	newServersRPC := make(chan *horus_net.ServerAddedMessage, horus_net.DefaultUnixSockRecvSize)
	newVCsRPC := make(chan *horus_net.VCUpdatedMessage, horus_net.DefaultUnixSockRecvSize)

	rpcEndPoint := horus_net.NewBareLeafRpcEndpoint(ctrlID,
		cfg.RemoteSrvServer,
		updatedServersRPC,
		newServersRPC,
		newVCsRPC)
	ch := NewLeafBusChan(hmEgress, updatedServersRPC, newServersRPC, newVCsRPC, asicIngress, asicEgress)

	return &leafController{
		rpcEndPoint: rpcEndPoint,
		ch:          ch,
		controller: &controller{
			ID:          ctrlID,
			cfg:         cfg,
			pipeID:      pipeID,
			asicIngress: asicIngress,
			asicEgress:  asicEgress,
		},
	}
}

func NewSpineController(ctrlID uint16, pipeID uint32, topoFp string, cfg *rootConfig) *spineController {
	topoCfg := model.ReadTopologyFile(topoFp)
	topology := model.NewDCNFromConf(topoCfg)
	vcm := core.NewVCManager(topology)

	asicEgress := make(chan []byte, horus_net.DefaultUnixSockSendSize)
	asicIngress := make(chan []byte, horus_net.DefaultUnixSockRecvSize)
	activeNode := make(chan *core.LeafHealthMsg, horus_net.DefaultUnixSockRecvSize)
	failedLeaves := make(chan *horus_net.LeafFailedMessage, horus_net.DefaultRpcRecvSize)
	failedServers := make(chan *horus_net.ServerFailedMessage, horus_net.DefaultRpcRecvSize)
	newLeaves := make(chan *horus_net.LeafAddedMessage, horus_net.DefaultRpcRecvSize)
	newServers := make(chan *horus_net.ServerAddedMessage, horus_net.DefaultRpcRecvSize)
	newVCs := make(chan *horus_net.VCUpdatedMessage, horus_net.DefaultRpcRecvSize)

	spine := topology.GetNode(ctrlID, model.NodeType_Spine)

	target := bfrtC.NewTarget(bfrtC.WithDeviceId(cfg.DeviceID), bfrtC.WithPipeId(pipeID))
	client := bfrtC.NewClient(cfg.BfrtAddress, cfg.P4Name, uint32(ctrlID), target)
	var cp SpineCP
	if cfg.ControlAPI == "fake" {
		cp = NewFakeSpineCP(spine)
	} else if cfg.ControlAPI == "v1" {
		cp = NewBfrtSpineCP_V1(client, spine)
	} else if cfg.ControlAPI == "v2" {
		cp = NewBfrtSpineCP_V2(client, spine)
	} else {
		logrus.Fatal("[Spine] Control API is invalid!")
	}

	rpcEndPoint := horus_net.NewSpineRpcEndpoint(spine.Address, cfg.RemoteSrvServer,
		topology, vcm, failedLeaves, failedServers, newLeaves, newServers, newVCs)
	ch := NewSpineBusChan(activeNode, failedLeaves, failedServers, newLeaves, newServers, newVCs,
		asicIngress, asicEgress)
	bus := NewSpineBus(ctrlID, ch, topology, vcm, nil, cp)
	return &spineController{
		rpcEndPoint: rpcEndPoint,
		healthMgr:   nil,
		bus:         bus,
		cp:          cp,
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
}

func (c *leafController) FetchTopology() error {
	go c.rpcEndPoint.StartClient()
	logrus.Debugf("[Leaf-%d] Fetching the current topology from %s", c.ID, c.rpcEndPoint.SrvCentralAddr)
	time.Sleep(time.Second)
	topoInfo, err := c.rpcEndPoint.GetTopology()
	if topoInfo == nil {
		logrus.Errorf("[Leaf-%d] No topology was fetched from %s", c.ID, c.rpcEndPoint.SrvCentralAddr)
	}
	if err != nil {
		logrus.Errorf(err.Error())
	}

	topo := model.NewDCNFromTopoInfo(topoInfo)
	c.topology = topo
	leaf, _ := c.topology.Leaves.Load(c.ID)
	if leaf == nil {
		logrus.Fatalf("[Leaf-%d] Leaf %d doesn't exist in the topology", c.ID, c.ID)
		return fmt.Errorf("leaf %d doesn't exist in the topology", c.ID)
	}

	// c.topology.Debug()

	c.vcm = core.NewVCManager(c.topology)
	c.rpcEndPoint.SetVCManager(c.vcm)
	c.rpcEndPoint.SetLocalAddress(leaf.Address)
	c.rpcEndPoint.SetTopology(c.topology)
	c.rpcEndPoint.StartServer()
	return nil
}

func (c *leafController) FetchVCs() error {
	logrus.Debugf("[Leaf-%d] Fetching all VCs from %s", c.ID, c.rpcEndPoint.SrvCentralAddr)
	time.Sleep(time.Second)
	vcs, err := c.rpcEndPoint.GetVCs()
	if len(vcs) == 0 {
		logrus.Warnf("[Leaf-%d] No VCs were fetched from %s", c.ID, c.rpcEndPoint.SrvCentralAddr)
	} else {
		logrus.Debugf("[Leaf-%d] %d VCs were fetched", c.ID, len(vcs))
	}
	if err != nil {
		logrus.Error(err)
		return err
	}

	for _, vcConf := range vcs {
		vc, err := model.NewVC(vcConf, c.topology)
		if err != nil {
			logrus.Error(err)
		} else {
			c.vcm.AddVC(vc)
		}
	}

	return nil
}

func (c *leafController) CreateHealthManager() error {
	healthMgr, err := core.NewLeafHealthManager(c.ID, c.bus.hmMsg, c.topology, c.vcm, c.cfg.Timeout)
	if err != nil {
		return err
	}
	c.healthMgr = healthMgr
	return nil
}

func (c *leafController) StartBare(fetchTopo bool) {
	logrus.
		WithFields(logrus.Fields{"ID": c.ID}).
		Infof("[Leaf] Starting leaf switch controller")
	c.controller.Start()

	if fetchTopo {
		err := c.FetchTopology()
		if err != nil {
			logrus.Fatal(err)
		}
	}

	err := c.FetchVCs()
	if err != nil {
		logrus.Error(err)
	}

	leaf := c.topology.GetNode(c.ID, model.NodeType_Leaf)
	target := bfrtC.NewTarget(bfrtC.WithDeviceId(c.cfg.DeviceID), bfrtC.WithPipeId(c.pipeID))
	client := bfrtC.NewClient(c.cfg.BfrtAddress, c.cfg.P4Name, uint32(c.ID), target)
	if c.cfg.ControlAPI == "fake" {
		c.cp = NewFakeLeafCP(leaf, c.cfg.Spines)
	} else if c.cfg.ControlAPI == "v1" {
		c.cp = NewBfrtLeafCP_V1(client, leaf, c.cfg.Spines)
	} else if c.cfg.ControlAPI == "v2" {
		c.cp = NewBfrtLeafCP_V2(client, leaf, c.cfg.Spines)
	} else {
		logrus.Fatal("[Leaf] Control API is invalid!")
	}

	c.bus = NewBareLeafBus(c.ID, c.ch, c.cp)
	err = c.CreateHealthManager()
	if err != nil {
		logrus.Fatal(err)
	}

	c.bus.SetHealthManager(c.healthMgr)
	c.bus.SetTopology(c.topology)
	c.bus.SetVCManager(c.vcm)
	// Init Table/register entries for each leaf
	c.cp.Init()
	go c.healthMgr.Start()
	go c.bus.Start()
}

func (c *leafController) Shutdown() {
	logrus.
		WithFields(logrus.Fields{"ID": c.ID}).
		Infof("[Leaf] Shutting down leaf switch controller")
	c.bus.DoneChan <- true
	c.healthMgr.DoneChan <- true
	c.rpcEndPoint.DoneChan <- true
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
	logrus.Debugf("[Spine] Fetching all VCs from %s", c.rpcEndPoint.SrvCentralAddr)
	time.Sleep(time.Second)
	vcs, err := c.rpcEndPoint.GetVCs()
	if len(vcs) == 0 {
		logrus.Warnf("[Spine] No VCs were fetched from %s", c.rpcEndPoint.SrvCentralAddr)
	} else {
		logrus.Debugf("[Spine] %d VCs were fetched", len(vcs))
	}
	if err != nil {
		logrus.Error(err)
	} else {
		for _, vcConf := range vcs {
			vc, err := model.NewVC(vcConf, c.topology)
			if err != nil {
				logrus.Error(err)
			} else {
				c.vcm.AddVC(vc)
			}
		}
	}

	go c.cp.Init()
	go c.bus.Start()
}
