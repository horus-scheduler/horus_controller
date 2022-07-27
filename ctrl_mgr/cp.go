package ctrl_mgr

import (
	"context"
	"encoding/binary"
	"fmt"
	"math"
	"net"

	"github.com/horus-scheduler/horus_controller/core"
	"github.com/horus-scheduler/horus_controller/core/model"
	bfrtC "github.com/khaledmdiab/bfrt-go-client/pkg/client"
	"github.com/sirupsen/logrus"
)

func updateOrInsert(ctx context.Context,
	ctrlStr string,
	client *bfrtC.Client,
	table string,
	ks []bfrtC.IKeyField,
	action string,
	ds []bfrtC.IDataField) error {
	var bfrtFn BfrtUpdateFn = nil
	entry := client.NewTableEntry(table, ks, action, ds, nil)
	// If the table has this entry -> Insert
	if ent, _ := client.ReadTableEntry(ctx, table, ks); ent != nil {
		logrus.Debugf("[%s] Table %s has an entry; key=%s. Updating this entry", ctrlStr, table, entry.GetKey().String())
		bfrtFn = client.ModifyTableEntry
	} else {
		logrus.Debugf("[%s] Table %s has NO entry; key=%s. Inserting a new entry", ctrlStr, table, entry.GetKey().String())
		bfrtFn = client.InsertTableEntry
	}

	if bfrtFn != nil {
		if err := bfrtFn(ctx, entry); err != nil {
			return fmt.Errorf("[%s] Setting up table %s failed. Error=%s", ctrlStr, table, err.Error())
		}
	} else {
		return fmt.Errorf("[%s] Setting up table %s failed. No update function is found", ctrlStr, table)
	}
	return nil
}

type ManagerCP interface {
	Init()
	InitRandomAdjustTables()
}

type LeafCP interface {
	Init()
	OnServerChange(*core.LeafHealthMsg)
}

type SpineCP interface {
	Init()
	InitRandomAdjustTables()
	OnLeafChange(uint64, uint64)
}

type FakeManagerCP struct {
}

func NewFakeManagerCP() *FakeManagerCP {
	return &FakeManagerCP{}
}

func (cp *FakeManagerCP) Init() {
	logrus.Info("Initializing FakeManagerCP")
}
func (cp *FakeManagerCP) InitRandomAdjustTables() {
	logrus.Info("Calling FakeManagerCP.InitRandomAdjustTables")
}

type FakeLeafCP struct {
	leaf   *model.Node
	spines []_SpineConfig
}

func NewFakeLeafCP(leaf *model.Node, spines []_SpineConfig) *FakeLeafCP {
	return &FakeLeafCP{
		leaf:   leaf,
		spines: spines,
	}
}

func (cp *FakeLeafCP) Init() {
	logrus.Infof("Initializing FakeLeafCP for Leaf %d", cp.leaf.ID)
}
func (cp *FakeLeafCP) OnServerChange(*core.LeafHealthMsg) {
	logrus.Infof("Calling FakeLeafCP.OnServerChange for Leaf %d", cp.leaf.ID)
}

type FakeSpineCP struct {
	spine *model.Node
}

func NewFakeSpineCP(spine *model.Node) *FakeSpineCP {
	return &FakeSpineCP{
		spine: spine,
	}
}
func (cp *FakeSpineCP) Init() {
	logrus.Info("Initializing FakeLeafCP")
}

func (cp *FakeSpineCP) InitRandomAdjustTables() {
	logrus.Info("Calling FakeSpineCP.InitRandomAdjustTables")
}
func (cp *FakeSpineCP) OnLeafChange(uint64, uint64) {
	logrus.Info("Calling FakeSpineCP.OnLeafChange")
}

type BfrtManagerCP_V1 struct {
	client *bfrtC.Client
}

func NewBfrtManagerCP_V1(client *bfrtC.Client) *BfrtManagerCP_V1 {
	return &BfrtManagerCP_V1{
		client: client,
	}
}

func (cp *BfrtManagerCP_V1) Init() {
	logrus.Info("Initializing BfrtManagerCP_V1")
	logrus.Debugf("[Manager] Setting up common leaf table entries")

	ctx := context.Background()

	bfrtclient := cp.client

	// Table entry for CPU port
	table := "pipe_leaf.LeafIngress.forward_saqr_switch_dst"
	action := "LeafIngress.act_forward_saqr"
	k1 := bfrtC.MakeExactKey("hdr.saqr.dst_id", uint64(model.CPU_PORT_ID))
	ks := bfrtC.MakeKeys(k1)
	d1 := bfrtC.MakeBytesData("port", uint64(model.CPU_PORT_ID))
	d2 := bfrtC.MakeBytesData("dst_mac", uint64(1000)) // Dummy
	ds := bfrtC.MakeData(d1, d2)
	entry := bfrtclient.NewTableEntry(table, ks, action, ds, nil)
	if err := bfrtclient.InsertTableEntry(ctx, entry); err != nil {
		logrus.Fatal(entry)
	}
	// Table entry for upstream port map (spine_id->port or client_id->port)
	for key, value := range model.UpstreamPortMap {
		table = "pipe_leaf.LeafIngress.forward_saqr_switch_dst"
		action = "LeafIngress.act_forward_saqr"
		k1 := bfrtC.MakeExactKey("hdr.saqr.dst_id", uint64(key))
		ks := bfrtC.MakeKeys(k1)
		d1 := bfrtC.MakeBytesData("port", uint64(value))
		d2 := bfrtC.MakeBytesData("dst_mac", uint64(1000+value)) // Dummy
		ds := bfrtC.MakeData(d1, d2)
		entry := bfrtclient.NewTableEntry(table, ks, action, ds, nil)
		if err := bfrtclient.InsertTableEntry(ctx, entry); err != nil {
			logrus.Fatal(entry)
		}
		// Table for Mirror functionality to send copy of original response packet
		table = "$mirror.cfg"
		action = "$normal"
		k1 = bfrtC.MakeExactKey("$sid", uint64(key))
		// TODO: Check correctness, data should be string "INGRESS", here we converted to uint64 to match the API
		dstr1 := bfrtC.MakeStrData("$direction", "INGRESS")
		d2 = bfrtC.MakeBytesData("$ucast_egress_port", uint64(value))
		d3 := bfrtC.MakeBoolData("$ucast_egress_port_valid", true)
		d4 := bfrtC.MakeBoolData("$session_enable", true)
		ks = bfrtC.MakeKeys(k1)
		ds = bfrtC.MakeData(dstr1, d2, d3, d4)
		entry = bfrtclient.NewTableEntry(table, ks, action, ds, nil)
		if err := bfrtclient.InsertTableEntry(ctx, entry); err != nil {
			logrus.Fatal(entry)
		}
	}

	cp.InitRandomAdjustTables()
}

func (cp *BfrtManagerCP_V1) InitRandomAdjustTables() {
	logrus.Info("Calling BfrtManagerCP.InitRandomAdjustTables")
	bfrtclient := cp.client
	ctx := context.Background()
	for i := 1; i <= 5; i++ {
		table_ds := "pipe_leaf.LeafIngress.adjust_random_range_ds"
		action := fmt.Sprintf("LeafIngress.adjust_random_worker_range_%d", i)
		k_ds_1 := bfrtC.MakeExactKey("saqr_md.cluster_num_valid_ds", uint64(math.Pow(2, float64(i))))
		k_ds := bfrtC.MakeKeys(k_ds_1)
		entry_ds := bfrtclient.NewTableEntry(table_ds, k_ds, action, nil, nil) // Parham: works with nil data?
		if err := bfrtclient.InsertTableEntry(ctx, entry_ds); err != nil {
			logrus.Fatal(entry_ds)
		}
		table_us := "pipe_leaf.LeafIngress.adjust_random_range_us"
		k_us_1 := bfrtC.MakeExactKey("saqr_md.cluster_num_valid_us", uint64(math.Pow(2, float64(i))))
		k_us := bfrtC.MakeKeys(k_us_1)
		entry_us := bfrtclient.NewTableEntry(table_us, k_us, action, nil, nil) // Parham: works with nil data?
		if err := bfrtclient.InsertTableEntry(ctx, entry_us); err != nil {
			logrus.Fatal(entry_us)
		}
	}
}

type BfrtLeafCP_V1 struct {
	client *bfrtC.Client
	leaf   *model.Node
	spines []_SpineConfig
}

func NewBfrtLeafCP_V1(client *bfrtC.Client, leaf *model.Node, spines []_SpineConfig) *BfrtLeafCP_V1 {
	return &BfrtLeafCP_V1{
		client: client,
		leaf:   leaf,
		spines: spines,
	}
}

func (cp *BfrtLeafCP_V1) Init() {
	logrus.Info("Initializing BfrtLeafCP_V1")
	ctx := context.Background()

	if cp.leaf == nil {
		logrus.Fatalf("[Leaf] Leaf doesn't exist")
	}

	spine := cp.leaf.Parent
	if spine == nil {
		logrus.Fatalf("[Leaf] Leaf %d has no parent", cp.leaf.ID)
	}

	leafIdx := cp.leaf.Index

	// Khaled: Currently, getting the spine Index isn't supported
	var spineIdx uint64 = 0

	logrus.Debugf("[Leaf] Setting up tables for leaf ID=%d, Index=%d", cp.leaf.ID, leafIdx)
	bfrtclient := cp.client

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
	for _, server := range cp.leaf.Children {
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

			err = updateOrInsert(ctx, "Leaf", bfrtclient, table, ks, action, ds)
			if err != nil {
				logrus.Fatal(err.Error())
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
	d2 := bfrtC.MakeBytesData("num_us_elements", uint64(len(cp.spines)))
	ks := bfrtC.MakeKeys(k1)
	ds := bfrtC.MakeData(d1, d2)
	err := updateOrInsert(ctx, "Leaf", bfrtclient, table, ks, action, ds)
	if err != nil {
		logrus.Fatal(err.Error())
	}

	// Table entries for port mapping of available upstream spines
	table = "pipe_leaf.LeafIngress.get_spine_dst_id"
	action = "LeafIngress.act_get_spine_dst_id"
	for spine_idx, spine := range cp.spines {
		k1 := bfrtC.MakeExactKey("saqr_md.random_id_1", uint64(spine_idx))
		k2 := bfrtC.MakeExactKey("hdr.saqr.cluster_id", uint64(leafIdx))
		ks := bfrtC.MakeKeys(k1, k2)
		d1 := bfrtC.MakeBytesData("spine_dst_id", uint64(spine.ID))
		ds := bfrtC.MakeData(d1)
		err := updateOrInsert(ctx, "Leaf", bfrtclient, table, ks, action, ds)
		if err != nil {
			logrus.Fatal(err.Error())
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
	err = updateOrInsert(ctx, "Leaf", bfrtclient, table, ks, action, ds)
	if err != nil {
		logrus.Fatal(err.Error())
	}
}

func (cp *BfrtLeafCP_V1) OnServerChange(hmMsg *core.LeafHealthMsg) {
	logrus.Debugf("[LeafBus-%d] Updating tables after server changes", cp.leaf.ID)

	ctx := context.Background()
	bfrtclient := cp.client
	worker_count := uint16(0)
	// Parham: findout number of alive workers
	for _, server := range cp.leaf.Children {
		worker_count += server.LastWorkerID - server.FirstWorkerID + 1
	}

	// Update entires #available workers
	table := "pipe_leaf.LeafIngress.get_cluster_num_valid"
	action := "LeafIngress.act_get_cluster_num_valid"
	// Parham: Assumed e.ctrlID is 0-indexed and indicates virtual leaf ID?
	k1 := bfrtC.MakeExactKey("hdr.saqr.cluster_id", uint64(cp.leaf.ID))
	d1 := bfrtC.MakeBytesData("num_ds_elements", uint64(worker_count))
	// Parham: How can we access number of spines avilable (random linkage for idle count),
	d2 := bfrtC.MakeBytesData("num_us_elements", uint64(2)) // put constant here works in our testbed but should be modified
	ks := bfrtC.MakeKeys(k1)
	ds := bfrtC.MakeData(d1, d2)
	err := updateOrInsert(ctx, "Leaf", bfrtclient, table, ks, action, ds)
	if err != nil {
		logrus.Fatal(err.Error())
	}

	// Update server port mappings
	table = "pipe_leaf.LeafIngress.forward_saqr_switch_dst"
	action = "LeafIngress.act_forward_saqr"
	for _, server := range hmMsg.Updated {
		for wid := server.FirstWorkerID; wid <= server.LastWorkerID; wid++ {
			// Parham: Assumed e.ctrlID is 0-indexed and indicates virtual leaf ID?
			index := uint16(cp.leaf.ID)*model.MAX_VCLUSTER_WORKERS + wid
			// Table entries for worker index to port mappings
			hw, err := net.ParseMAC(server.Address)
			// Parham: TODO: Check endian not sure how bfrt client converted MACs originally
			mac_data := binary.LittleEndian.Uint64(hw)
			if err != nil {
				logrus.Fatal(err)
			}
			k1 := bfrtC.MakeExactKey("hdr.saqr.dst_id", uint64(index))
			ks := bfrtC.MakeKeys(k1)
			d1 := bfrtC.MakeBytesData("port", uint64(server.PortId))
			d2 := bfrtC.MakeBytesData("dst_mac", mac_data)
			ds := bfrtC.MakeData(d1, d2)
			err = updateOrInsert(ctx, "Leaf", bfrtclient, table, ks, action, ds)
			if err != nil {
				logrus.Fatal(err.Error())
			}
		}
	}
	// Update qlen unit
	qlen_unit, _ := model.WorkerQlenUnitMap[worker_count]
	table = "pipe_leaf.LeafIngress.set_queue_len_unit"
	action = "LeafIngress.act_set_queue_len_unit"
	k1 = bfrtC.MakeExactKey("hdr.saqr.cluster_id", uint64(cp.leaf.ID))
	d1 = bfrtC.MakeBytesData("hdr.saqr.cluster_id", uint64(qlen_unit))
	ks = bfrtC.MakeKeys(k1)
	ds = bfrtC.MakeData(d1)
	err = updateOrInsert(ctx, "Leaf", bfrtclient, table, ks, action, ds)
	if err != nil {
		logrus.Fatal(err.Error())
	}
}

type BfrtSpineCP_V1 struct {
	client *bfrtC.Client
	spine  *model.Node
}

func NewBfrtSpineCP_V1(client *bfrtC.Client, spine *model.Node) *BfrtSpineCP_V1 {
	return &BfrtSpineCP_V1{
		client: client,
		spine:  spine,
	}
}

func (cp *BfrtSpineCP_V1) Init() {
	logrus.Info("Initializing BfrtSpineCP_V1")
	ctx := context.Background()
	bfrtclient := cp.client
	// Parham: Is controller ID 0 indexed?
	spine := cp.spine
	if spine == nil {
		logrus.Fatalf("[Spine] Spine doesn't exist")
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

		// Mapping leafID -> index (of qlen list)
		table = "pipe_spine.SpineIngress.get_switch_index"
		action = "SpineIngress.act_get_switch_index"
		k1 = bfrtC.MakeExactKey("hdr.saqr.cluster_id", vcID)
		k2 = bfrtC.MakeExactKey("hdr.saqr.src_id", uint64(leaf.ID))
		d1 = bfrtC.MakeBytesData("switch_index", uint64(i))
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

	cp.InitRandomAdjustTables()
}

func (cp *BfrtSpineCP_V1) InitRandomAdjustTables() {
	logrus.Info("Calling BfrtSpineCP_V1.InitRandomAdjustTables")
	ctx := context.Background()
	bfrtclient := cp.client
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
func (cp *BfrtSpineCP_V1) OnLeafChange(leafID uint64, index uint64) {
	logrus.Info("Calling BfrtSpineCP_V1.OnLeafChange")
	logrus.Debugf("[SpineBus-%d] Updating tables after leaf changes", cp.spine.ID)
	spine := cp.spine

	INVALID_VAL_16bit := uint64(0x7FFF)
	ctx := context.Background()
	bfrtclient := cp.client
	vcID := uint64(0)
	idleCount := uint64(0)

	regIdleCount := "pipe_spine.SpineIngress.idle_count"
	val, err1 := bfrtclient.ReadRegister(ctx, regIdleCount, vcID)
	idleCount = val
	if err1 != nil {
		logrus.Fatal("Cannot read register")
	}

	regIdleIdxMap := "pipe_spine.SpineIngress.idle_list_idx_mapping"
	indexAtIdleList, err1 := bfrtclient.ReadRegister(ctx, regIdleIdxMap, leafID)
	if err1 != nil {
		logrus.Fatal("Cannot read register")
	}
	if indexAtIdleList != INVALID_VAL_16bit { // Failed leaf was in the idle list of spine
		logrus.Debugf("[SpineBus] failed leaf was in idle list at index %d", indexAtIdleList)
		rentry := bfrtclient.NewRegisterEntry(regIdleIdxMap, leafID, INVALID_VAL_16bit, nil) // write invalid val on the mapping reg
		if err := bfrtclient.InsertTableEntry(ctx, rentry); err != nil {
			logrus.Fatalf("[SpineBus] Writing on register %s failed", regIdleIdxMap)
		}
		if indexAtIdleList < idleCount-1 { // Read Idle list and swap write the last element on the index of the recently failed leaf
			// Read last idle list element
			regIdleList := "pipe_spine.SpineIngress.idle_list"
			lastElement, _ := bfrtclient.ReadRegister(ctx, regIdleList, idleCount-1)
			rentry = bfrtclient.NewRegisterEntry(regIdleList, indexAtIdleList, lastElement, nil) // write last element on new index
			if err := bfrtclient.InsertTableEntry(ctx, rentry); err != nil {
				logrus.Fatalf("[SpineBus] Writing on register %s failed", regIdleList)
			}
		}
		// Decrement idle count
		rentry = bfrtclient.NewRegisterEntry(regIdleCount, vcID, idleCount-1, nil)
	}

	// Decrement total number of available children
	table := "pipe_spine.SpineIngress.get_cluster_num_valid_leafs"
	action := "SpineIngress.act_get_cluster_num_valid_leafs"
	k1 := bfrtC.MakeExactKey("hdr.saqr.cluster_id", vcID)
	d1 := bfrtC.MakeBytesData("num_leafs", uint64(len(spine.Children)))
	ks := bfrtC.MakeKeys(k1)
	ds := bfrtC.MakeData(d1)
	err := updateOrInsert(ctx, "Spine", bfrtclient, table, ks, action, ds)
	if err != nil {
		logrus.Fatal(err.Error())
	}

	// Copy and shift to left cell the queue len lists for every index i where i < falied leaf index
	// Parham: We need to check since these are being modified by data plane it might be already updated by leaf and this move
	// results in incorrect state until next queue signal arrives from the leaf
	for i := int(index) + 1; i <= len(spine.Children); i++ {
		qlenList1 := "pipe_spine.SpineIngress.queue_len_list_1"
		qlenList2 := "pipe_spine.SpineIngress.queue_len_list_2"
		defList1 := "pipe_spine.SpineIngress.deferred_queue_len_list_1"
		defList2 := "pipe_spine.SpineIngress.deferred_queue_len_list_2"

		nextCellQlen, _ := bfrtclient.ReadRegister(ctx, qlenList1, uint64(i))
		nextCellDrift, _ := bfrtclient.ReadRegister(ctx, defList1, uint64(i))

		rentry := bfrtclient.NewRegisterEntry(qlenList1, uint64(i-1), nextCellQlen+nextCellDrift, nil)
		if err := bfrtclient.InsertTableEntry(ctx, rentry); err != nil {
			logrus.Fatalf("[SpineBus] Writing on register %s failed", qlenList1)
		}
		rentry = bfrtclient.NewRegisterEntry(qlenList2, uint64(i-1), nextCellQlen+nextCellDrift, nil)
		if err := bfrtclient.InsertTableEntry(ctx, rentry); err != nil {
			logrus.Fatalf("[SpineBus] Writing on register %s failed", qlenList2)
		}

		// reset drift
		rentry = bfrtclient.NewRegisterEntry(defList1, uint64(i-1), 0, nil)
		if err := bfrtclient.InsertTableEntry(ctx, rentry); err != nil {
			logrus.Fatalf("[SpineBus] Writing on register %s failed", defList1)
		}
		rentry = bfrtclient.NewRegisterEntry(defList2, uint64(i-1), 0, nil)
		if err := bfrtclient.InsertTableEntry(ctx, rentry); err != nil {
			logrus.Fatalf("[SpineBus] Writing on register %s failed", defList2)
		}
	}

	// Parham: Assuming leaf IDs stay the same after failure but leaf indices were updated accordingly
	for _, leaf := range spine.Children {
		// index (of qlen list) -> leafID mapping
		table := "pipe_spine.SpineIngress.get_rand_leaf_id_1"
		action := "SpineIngress.act_get_rand_leaf_id_1"
		k1 := bfrtC.MakeExactKey("saqr_md.random_ds_index_1", uint64(leaf.Index))
		k2 := bfrtC.MakeExactKey("hdr.saqr.cluster_id", vcID)
		d1 = bfrtC.MakeBytesData("leaf_id", uint64(leaf.ID))
		ks = bfrtC.MakeKeys(k1, k2)
		ds = bfrtC.MakeData(d1)
		err := updateOrInsert(ctx, "Spine", bfrtclient, table, ks, action, ds)
		if err != nil {
			logrus.Fatal(err.Error())
		}

		table = "pipe_spine.SpineIngress.get_rand_leaf_id_2"
		action = "SpineIngress.act_get_rand_leaf_id_2"
		k1 = bfrtC.MakeExactKey("saqr_md.random_ds_index_2", uint64(leaf.Index))
		k2 = bfrtC.MakeExactKey("hdr.saqr.cluster_id", vcID)
		d1 = bfrtC.MakeBytesData("leaf_id", uint64(leaf.ID))
		ks = bfrtC.MakeKeys(k1, k2)
		ds = bfrtC.MakeData(d1)
		err = updateOrInsert(ctx, "Spine", bfrtclient, table, ks, action, ds)
		if err != nil {
			logrus.Fatal(err.Error())
		}

		// Mapping leafID -> index (of qlen list)
		table = "pipe_spine.SpineIngress.get_switch_index"
		action = "SpineIngress.act_get_switch_index"
		k1 = bfrtC.MakeExactKey("hdr.saqr.cluster_id", vcID)
		k2 = bfrtC.MakeExactKey("hdr.saqr.src_id", uint64(leaf.ID))
		d1 = bfrtC.MakeBytesData("switch_index", uint64(leaf.Index))
		ks = bfrtC.MakeKeys(k1, k2)
		ds = bfrtC.MakeData(d1)
		err = updateOrInsert(ctx, "Spine", bfrtclient, table, ks, action, ds)
		if err != nil {
			logrus.Fatal(err.Error())
		}
	}
}
