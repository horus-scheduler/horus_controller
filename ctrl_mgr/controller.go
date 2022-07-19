package ctrl_mgr

import (
	"context"
	"encoding/binary"
	"fmt"
	"math"
	"net"
	"time"

	"github.com/horus-scheduler/horus_controller/core"
	"github.com/horus-scheduler/horus_controller/core/model"
	horus_net "github.com/horus-scheduler/horus_controller/core/net"
	horus_pb "github.com/horus-scheduler/horus_controller/protobuf"
	bfrtC "github.com/khaledmdiab/bfrt-go-client/pkg/client"
	"github.com/sirupsen/logrus"
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

type TopologyOption func(*model.Topology) error

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

	target := bfrtC.NewTarget(bfrtC.WithDeviceId(cfg.DeviceID), bfrtC.WithPipeId(pipeID))
	bfrt := bfrtC.NewClient(cfg.BfrtAddress, cfg.P4Name, uint32(ctrlID), target)

	rpcEndPoint := horus_net.NewBareLeafRpcEndpoint(ctrlID,
		cfg.RemoteSrvServer,
		updatedServersRPC,
		newServersRPC,
		newVCsRPC)
	ch := NewLeafBusChan(hmEgress, updatedServersRPC, newServersRPC, newVCsRPC, asicIngress, asicEgress)
	bus := NewBareLeafBus(ctrlID, ch, bfrt)
	return &leafController{
		rpcEndPoint: rpcEndPoint,
		bus:         bus,
		controller: &controller{
			ID:          ctrlID,
			cfg:         cfg,
			bfrt:        bfrt,
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
	bfrt := bfrtC.NewClient(cfg.BfrtAddress, cfg.P4Name, uint32(spine.ID), target)

	rpcEndPoint := horus_net.NewSpineRpcEndpoint(spine.Address, cfg.RemoteSrvServer,
		topology, vcm, failedLeaves, failedServers, newLeaves, newServers, newVCs)
	ch := NewSpineBusChan(activeNode, failedLeaves, failedServers, newLeaves, newServers, newVCs,
		asicIngress, asicEgress)
	bus := NewSpineBus(ctrlID, ch, topology, vcm, nil, bfrt)
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

func (c *leafController) init_leaf_bfrt_setup() {
	ctx := context.Background()

	// Parham: Is controller ID 0 indexed?
	topology := c.topology
	leaf := topology.GetNode(c.ID, model.NodeType_Leaf)
	if leaf == nil {
		logrus.Fatalf("[Leaf] Leaf %d doesn't exist", c.ID)
	}

	spine := leaf.Parent
	if spine == nil {
		logrus.Fatalf("[Leaf] Leaf %d has no parent", c.ID)
	}

	leafIdx := leaf.Index

	// Khaled: Currently, getting the spine Index isn't supported
	var spineIdx uint64 = 0

	logrus.Debugf("[Leaf] Setting up tables for leaf ID=%d, Index=%d", leaf.ID, leafIdx)
	bfrtclient := c.bfrt

	/*
		Parham: c.cfg.SpineIDs[0] bad practice but should work in our testbed. In real-world this should come from central ctrl
		TODO: Read from topology model and get parent of this leaf(?)
	*/
	reg := "pipe_leaf.LeafIngress.linked_iq_sched"
	rentry := bfrtclient.NewRegisterEntry(reg, uint64(leafIdx), spineIdx, nil)
	if err := bfrtclient.InsertTableEntry(ctx, rentry); err != nil {
		logrus.Fatalf("[Leaf] Setting up register %s failed", reg)
	}
	reg = "pipe_leaf.LeafIngress.linked_sq_sched"
	rentry = bfrtclient.NewRegisterEntry(reg, uint64(leafIdx), spineIdx, nil)
	if err := bfrtclient.InsertTableEntry(ctx, rentry); err != nil {
		logrus.Fatalf("[Leaf] Setting up register %s failed", reg)
	}

	worker_count := uint16(0)

	// Insert idle_list values (wid of idle workers) and table entries for wid to port mapping
	reg = "pipe_leaf.LeafIngress.idle_list"
	table := "pipe_leaf.LeafIngress.forward_saqr_switch_dst"
	action := "LeafIngress.act_forward_saqr"
	for _, server := range leaf.Children {
		// Parham: Check this line seems redundant, how can we access the worker count of server
		worker_count += server.LastWorkerID - server.FirstWorkerID + 1
		for wid := server.FirstWorkerID; wid <= server.LastWorkerID; wid++ {
			index := leafIdx*model.MAX_VCLUSTER_WORKERS + wid
			// logrus.Info("leafIdx: ", leafIdx, ", calc_index: ", index, ", wid: ", wid)
			// TODO: Check wid logic, we assume each leaf has workers 0 indexed
			// but for virt. implementation we assign wids: [0,n] for leaf1, [n+1-m] for leaf2 ...
			rentry := bfrtclient.NewRegisterEntry(reg, uint64(index), uint64(index), nil)
			if err := bfrtclient.InsertTableEntry(ctx, rentry); err != nil {
				logrus.Errorf("[Leaf] Setting up register %s at index = %d failed", reg, index)
				logrus.Fatal(rentry)
			}

			// Table entries for worker index to port mappings
			hw, err := net.ParseMAC(server.Address)
			// Parham: TODO: Check endian not sure how bfrt client converted MACs originally
			hw = append(hw, make([]byte, 8-len(hw))...)
			mac_data := binary.LittleEndian.Uint64(hw)
			if err != nil {
				logrus.Fatal(err)
			}
			k1 := bfrtC.MakeExactKey("hdr.saqr.dst_id", uint64(index))
			ks := bfrtC.MakeKeys(k1) // Parham: is this needed even for single key?
			d1 := bfrtC.MakeBytesData("port", uint64(server.PortId))
			d2 := bfrtC.MakeBytesData("dst_mac", mac_data)
			ds := bfrtC.MakeData(d1, d2)
			entry := bfrtclient.NewTableEntry(table, ks, action, ds, nil)
			if err := bfrtclient.InsertTableEntry(ctx, entry); err != nil {
				logrus.Errorf("[Leaf] Setting up table %s failed", table)
				logrus.Fatal(entry)
			}
		}
	}

	// Register entry #idle workers
	reg = "pipe_leaf.LeafIngress.idle_count"
	rentry = bfrtclient.NewRegisterEntry(reg, uint64(leafIdx), uint64(worker_count), nil)
	if err := bfrtclient.InsertTableEntry(ctx, rentry); err != nil {
		logrus.Fatalf("[Leaf] Setting up register %s failed", reg)
	}

	// Table entires for #available workers and #available spine schedulers
	// Khaled: This should be per VC?
	table = "pipe_leaf.LeafIngress.get_cluster_num_valid"
	action = "LeafIngress.act_get_cluster_num_valid"
	k1 := bfrtC.MakeExactKey("hdr.saqr.cluster_id", uint64(leafIdx))
	d1 := bfrtC.MakeBytesData("num_ds_elements", uint64(worker_count))
	d2 := bfrtC.MakeBytesData("num_us_elements", uint64(len(c.cfg.Spines)))
	ks := bfrtC.MakeKeys(k1)
	ds := bfrtC.MakeData(d1, d2)
	entry := bfrtclient.NewTableEntry(table, ks, action, ds, nil)
	if err := bfrtclient.InsertTableEntry(ctx, entry); err != nil {
		logrus.Errorf("[Leaf] Setting up table %s failed", table)
		logrus.Fatal(entry)
	}

	// Table entries for port mapping of available upstream spines
	table = "pipe_leaf.LeafIngress.get_spine_dst_id"
	action = "LeafIngress.act_get_spine_dst_id"
	for spine_idx, spine := range c.cfg.Spines {
		k1 := bfrtC.MakeExactKey("saqr_md.random_id_1", uint64(spine_idx))
		k2 := bfrtC.MakeExactKey("hdr.saqr.cluster_id", uint64(leafIdx))
		ks := bfrtC.MakeKeys(k1, k2)
		d1 := bfrtC.MakeBytesData("spine_dst_id", uint64(spine.ID))
		ds := bfrtC.MakeData(d1)
		entry := bfrtclient.NewTableEntry(table, ks, action, ds, nil)
		if err := bfrtclient.InsertTableEntry(ctx, entry); err != nil {
			logrus.Errorf("[Leaf] Setting up table %s failed", table)
			logrus.Fatal(entry)
		}
	}

	// Table entries for qlen unit based on #workers (used for avg calc.)
	qlen_unit := model.WorkerQlenUnitMap[worker_count]
	table = "pipe_leaf.LeafIngress.set_queue_len_unit"
	action = "LeafIngress.act_set_queue_len_unit"
	k1 = bfrtC.MakeExactKey("hdr.saqr.cluster_id", uint64(leafIdx))
	d1 = bfrtC.MakeBytesData("cluster_unit", uint64(qlen_unit))
	ks = bfrtC.MakeKeys(k1)
	ds = bfrtC.MakeData(d1)
	entry = bfrtclient.NewTableEntry(table, ks, action, ds, nil)
	if err := bfrtclient.InsertTableEntry(ctx, entry); err != nil {
		logrus.Errorf("[Leaf] Setting up table %s failed", table)
		logrus.Fatal(entry)
	}
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

	err = c.CreateHealthManager()
	if err != nil {
		logrus.Fatal(err)
	}

	c.bus.SetHealthManager(c.healthMgr)
	c.bus.SetTopology(c.topology)
	c.bus.SetVCManager(c.vcm)

	// Init Table/register entries for each leaf
	c.init_leaf_bfrt_setup()
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

func (c *spineController) init_random_adjust_tables() {
	ctx := context.Background()
	bfrtclient := c.bfrt
	for i := 1; i <= 3; i++ {
		table_ds := "pipe_spine.SpineIngress.adjust_random_range_sq_leafs"
		keyValue := uint64(math.Pow(2, float64(i)))
		action := fmt.Sprintf("SpineIngress.adjust_random_leaf_index_%d", keyValue)
		k_ds_1 := bfrtC.MakeExactKey("saqr_md.cluster_num_valid_queue_signals", keyValue)
		k_ds := bfrtC.MakeKeys(k_ds_1)
		// logrus.Debugf("i=%d, key=%d, action=%s", i, uint64(math.Pow(2, float64(i))), action)
		entry_ds := bfrtclient.NewTableEntry(table_ds, k_ds, action, nil, nil) // Parham: works with nil data?
		if err := bfrtclient.InsertTableEntry(ctx, entry_ds); err != nil {
			logrus.Error(err.Error())
			logrus.Fatal(entry_ds)
		}
	}
}

func (c *spineController) init_spine_bfrt_setup() {
	ctx := context.Background()
	bfrtclient := c.bfrt
	// Parham: Is controller ID 0 indexed?
	topology := c.topology
	spine := topology.GetNode(c.ID, model.NodeType_Spine)
	if spine == nil {
		logrus.Fatalf("[Spine] Spine %d doesn't exist", c.ID)
	}
	// Parham: assuming single VC for now TODO: check and fix later
	var vcID uint64 = 0

	for i, leaf := range spine.Children {
		//leafIdx := leaf.Index
		worker_count := uint16(0)
		for _, server := range leaf.Children {
			// Parham: Check this line seems redundant, how can we access the worker count of server
			worker_count += server.LastWorkerID - server.FirstWorkerID + 1
		}

		// Initialize idle list and add all children (leaves)
		reg := "pipe_spine.SpineIngress.idle_list"
		rentry := bfrtclient.NewRegisterEntry(reg, uint64(i), uint64(leaf.ID), nil)
		if err := bfrtclient.InsertTableEntry(ctx, rentry); err != nil {
			logrus.Error(err.Error())
			logrus.Fatalf("[Spine] Setting up register %s failed", reg)
		}

		// mapping[leafID] -> index of leaf in idle_list.
		reg = "pipe_spine.SpineIngress.idle_list_idx_mapping"
		rentry = bfrtclient.NewRegisterEntry(reg, uint64(leaf.ID), uint64(i), nil)
		if err := bfrtclient.InsertTableEntry(ctx, rentry); err != nil {
			logrus.Fatalf("[Spine] Setting up register %s failed", reg)
		}

		// Leaf port mapping
		table := "pipe_spine.SpineIngress.forward_saqr_switch_dst"
		// Parham: action doesn't need to start with pipe name? E.g, pipe_spine.Spineingress....
		action := "SpineIngress.act_forward_saqr"
		k1 := bfrtC.MakeExactKey("hdr.saqr.dst_id", uint64(leaf.ID))
		d1 := bfrtC.MakeBytesData("port", uint64(leaf.PortId))
		ks := bfrtC.MakeKeys(k1)
		ds := bfrtC.MakeData(d1)
		entry := bfrtclient.NewTableEntry(table, ks, action, ds, nil)
		if err := bfrtclient.InsertTableEntry(ctx, entry); err != nil {
			logrus.Errorf("[Spine] Setting up table %s failed", table)
			logrus.Fatal(entry)
		}

		// LeafId -> qlen_unit mapping (two tables since we have two samples)
		table = "pipe_spine.SpineIngress.set_queue_len_unit_1"
		action = "SpineIngress.act_set_queue_len_unit_1"
		k1 = bfrtC.MakeExactKey("saqr_md.random_id_1", uint64(leaf.ID))
		k2 := bfrtC.MakeExactKey("hdr.saqr.cluster_id", vcID)
		d1 = bfrtC.MakeBytesData("cluster_unit", uint64(model.WorkerQlenUnitMap[worker_count]))
		ks = bfrtC.MakeKeys(k1, k2)
		ds = bfrtC.MakeData(d1)
		entry = bfrtclient.NewTableEntry(table, ks, action, ds, nil)
		if err := bfrtclient.InsertTableEntry(ctx, entry); err != nil {
			logrus.Errorf("[Spine] Setting up table %s failed", table)
			logrus.Fatal(entry)
		}
		table = "pipe_spine.SpineIngress.set_queue_len_unit_2"
		action = "SpineIngress.act_set_queue_len_unit_2"
		k1 = bfrtC.MakeExactKey("saqr_md.random_id_2", uint64(leaf.ID))
		k2 = bfrtC.MakeExactKey("hdr.saqr.cluster_id", vcID)
		d1 = bfrtC.MakeBytesData("cluster_unit", uint64(model.WorkerQlenUnitMap[worker_count]))
		ks = bfrtC.MakeKeys(k1, k2)
		ds = bfrtC.MakeData(d1)
		entry = bfrtclient.NewTableEntry(table, ks, action, ds, nil)
		if err := bfrtclient.InsertTableEntry(ctx, entry); err != nil {
			logrus.Errorf("[Spine] Setting up table %s failed", table)
			logrus.Fatal(entry)
		}

		// index (of qlen list) -> leafID mapping
		table = "pipe_spine.SpineIngress.get_rand_leaf_id_1"
		action = "SpineIngress.act_get_rand_leaf_id_1"
		k1 = bfrtC.MakeExactKey("saqr_md.random_ds_index_1", uint64(i))
		k2 = bfrtC.MakeExactKey("hdr.saqr.cluster_id", vcID)
		d1 = bfrtC.MakeBytesData("leaf_id", uint64(leaf.ID))
		ks = bfrtC.MakeKeys(k1, k2)
		ds = bfrtC.MakeData(d1)
		entry = bfrtclient.NewTableEntry(table, ks, action, ds, nil)
		if err := bfrtclient.InsertTableEntry(ctx, entry); err != nil {
			logrus.Errorf("[Spine] Setting up table %s failed", table)
			logrus.Fatal(entry)
		}
		table = "pipe_spine.SpineIngress.get_rand_leaf_id_2"
		action = "SpineIngress.act_get_rand_leaf_id_2"
		k1 = bfrtC.MakeExactKey("saqr_md.random_ds_index_2", uint64(i))
		k2 = bfrtC.MakeExactKey("hdr.saqr.cluster_id", vcID)
		d1 = bfrtC.MakeBytesData("leaf_id", uint64(leaf.ID))
		ks = bfrtC.MakeKeys(k1, k2)
		ds = bfrtC.MakeData(d1)
		entry = bfrtclient.NewTableEntry(table, ks, action, ds, nil)
		if err := bfrtclient.InsertTableEntry(ctx, entry); err != nil {
			logrus.Errorf("[Spine] Setting up table %s failed", table)
			logrus.Fatal(entry)
		}
	}

	reg := "pipe_spine.SpineIngress.idle_count"
	rentry := bfrtclient.NewRegisterEntry(reg, uint64(0), uint64(len(spine.Children)), nil)
	if err := bfrtclient.InsertTableEntry(ctx, rentry); err != nil {
		logrus.Fatalf("[Spine] Setting up register %s failed", reg)
	}

	// vcID -> #leaves mapping
	table := "pipe_spine.SpineIngress.get_cluster_num_valid_leafs"
	action := "SpineIngress.act_get_cluster_num_valid_leafs"
	k1 := bfrtC.MakeExactKey("hdr.saqr.cluster_id", vcID)
	d1 := bfrtC.MakeBytesData("num_leafs", uint64(len(spine.Children)))
	ks := bfrtC.MakeKeys(k1)
	ds := bfrtC.MakeData(d1)
	entry := bfrtclient.NewTableEntry(table, ks, action, ds, nil)
	if err := bfrtclient.InsertTableEntry(ctx, entry); err != nil {
		logrus.Errorf("[Spine] Setting up table %s failed", table)
		logrus.Fatal(entry)
	}

	c.init_random_adjust_tables()

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

	go c.init_spine_bfrt_setup()
	go c.bus.Start()
}
