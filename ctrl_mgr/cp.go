package ctrl_mgr

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"math"
	"net"
	"time"

	"github.com/horus-scheduler/horus_controller/core/model"
	bfrtC "github.com/khaledmdiab/bfrt-go-client/pkg/client"
	"github.com/sirupsen/logrus"
)

const INVALID_VAL_16bit uint64 = (0x7FFF)

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

func LeafCPInitAllVer(ctx context.Context,
	leaf *model.Node,
	bfrtclient *bfrtC.Client,
	spines []_SpineConfig,
	topology *model.Topology) {
	logrus.Info("Initializing Leaf CP common for all Versions")

	spine := leaf.Parent
	if spine == nil {
		logrus.Fatalf("[Leaf] Leaf %d has no parent", leaf.ID)
	}

	leafIdx := leaf.Index

	// UpstreamIDs: IDs for the Spines and Clients
	var upstreamIDs []int
	for _, sp := range topology.Spines.Internal() {
		upstreamIDs = append(upstreamIDs, int(sp.ID))
	}
	for _, client := range topology.Clients.Internal() {
		upstreamIDs = append(upstreamIDs, int(client.ID))
	}

	// Khaled: Currently, getting the spine Index isn't supported
	//var spineIdx uint64 = 0

	logrus.Debugf("[Leaf] Setting up tables for leaf ID=%d, Index=%d", leaf.ID, leafIdx)

	worker_count := uint16(0)

	// Insert idle_list values (wid of idle workers) and table entries for wid to port mapping
	reg := "pipe_leaf.LeafIngress.idle_list"
	table := "pipe_leaf.LeafIngress.forward_horus_switch_dst"
	action := "LeafIngress.act_forward_horus"
	logrus.Debugf("[Leaf-%d] #children=%d", leaf.ID,  len(leaf.Children))
	for _, server := range leaf.Children {
		// Parham: Check this line seems redundant, how can we access the worker count of server
		logrus.Debugf("[Leaf-%d] leafID %d, server %d first-wid=%d, last-wid=%d", leafIdx, leaf.ID, server.ID, server.FirstWorkerID, server.LastWorkerID)
		worker_count += server.LastWorkerID - server.FirstWorkerID + 1
		for wid := server.FirstWorkerID; wid <= server.LastWorkerID; wid++ {
			index := leaf.ID*model.MAX_VCLUSTER_WORKERS + wid
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
			hw = append(make([]byte, 8-len(hw)), hw...)
			//logrus.Debugf("[Leaf-%d] index %d hw %s", c.ID, index, hw)
			mac_data := binary.BigEndian.Uint64(hw)

			if err != nil {
				logrus.Fatal(err)
			}
			k1 := bfrtC.MakeExactKey("hdr.horus.dst_id", uint64(index))
			k2 := bfrtC.MakeExactKey("hdr.horus.pool_id", uint64(leaf.ID))
			ks := bfrtC.MakeKeys(k1, k2)
			// Khaled: Check PortId -> UsPort.GetDevPort()
			// d1 := bfrtC.MakeBytesData("port", uint64(server.PortId))
			d1 := bfrtC.MakeBytesData("port", uint64(server.Port.GetDevPort()))
			d2 := bfrtC.MakeBytesData("dst_mac", mac_data)
			ds := bfrtC.MakeData(d1, d2)

			err = updateOrInsert(ctx, "Leaf", bfrtclient, table, ks, action, ds)
			if err != nil {
				logrus.Fatal(err.Error())
			}
		}
	}
	for _, uid := range upstreamIDs {
		// Table for Mirror functionality to send copy of original response packet
		table = "$mirror.cfg"
		action = "$normal"
		key := uint64(uid + int(leaf.Index)) // Each emulated leaf will use a seperate port for mirror
		k1 := bfrtC.MakeExactKey("$sid", key)
		dstr1 := bfrtC.MakeStrData("$direction", "INGRESS")
		// Each emulated leaf will use a seperate port for mirror
		// d2 := bfrtC.MakeBytesData("$ucast_egress_port", uint64(model.LeafUpstreamPortMap[leaf.Index]))
		// Khaled: Check LeafUpstreamPortMap[..] -> leaf.UsPort.GetDevPort()
		usPortId := leaf.UsPort.GetDevPort()
		d2 := bfrtC.MakeBytesData("$ucast_egress_port", uint64(usPortId))
		d3 := bfrtC.MakeBoolData("$ucast_egress_port_valid", true)
		d4 := bfrtC.MakeBoolData("$session_enable", true)
		ks := bfrtC.MakeKeys(k1)
		ds := bfrtC.MakeData(dstr1, d2, d3, d4)
		err := updateOrInsert(ctx, "Leaf", bfrtclient, table, ks, action, ds)
		if err != nil {
			logrus.Debugf("Error in Mirror table entry: leaf %d, key %d, session: %d", leaf.Index, key, usPortId)
			logrus.Fatal(err)
		}
		//entry := bfrtclient.NewTableEntry(table, ks, action, ds, nil)
		//if err := bfrtclient.InsertTableEntry(ctx, entry); err != nil {
		//	logrus.Debugf("Error in Mirror table entry: leaf %d, key %d, session: %d", leaf.Index, key, usPortId)
		//	logrus.Fatal(entry)
		//}

		table = "pipe_leaf.LeafIngress.forward_horus_switch_dst"
		action = "LeafIngress.act_forward_horus"
		k1 = bfrtC.MakeExactKey("hdr.horus.dst_id", uint64(uid))
		k2 := bfrtC.MakeExactKey("hdr.horus.pool_id", uint64(leaf.Index))
		ks = bfrtC.MakeKeys(k1, k2)
		usPort := leaf.UsPort.GetDevPort()
		d1 := bfrtC.MakeBytesData("port", usPort)
		d2 = bfrtC.MakeBytesData("dst_mac", uint64(100)) // Dummy mac address for port
		ds = bfrtC.MakeData(d1, d2)
		err = updateOrInsert(ctx, "Leaf", bfrtclient, table, ks, action, ds)
		if err != nil {
			logrus.Fatal(err.Error())
		}

	}

	// Register entry #idle workers
	reg = "pipe_leaf.LeafIngress.idle_count"
	rentry := bfrtclient.NewRegisterEntry(reg, uint64(leaf.ID), uint64(worker_count), nil)
	if err := bfrtclient.InsertTableEntry(ctx, rentry); err != nil {
		logrus.Fatalf("[Leaf] Setting up register %s failed", reg)
	}

	// New idleRemove approach
	// reg = "pipe_leaf.LeafIngress.linked_iq_index"
	// rentry = bfrtclient.NewRegisterEntry(reg, uint64(leaf.ID), uint64(leafIdx), nil)
	// if err := bfrtclient.InsertTableEntry(ctx, rentry); err != nil {
	// 	logrus.Fatalf("[Leaf] Setting up register %s failed", reg)
	// }

	// Table entires for #available workers and #available spine schedulers
	// Khaled: This should be per VC?
	num_spines := int(math.Max(float64(len(spines)), 2))
	table = "pipe_leaf.LeafIngress.get_cluster_num_valid"
	action = "LeafIngress.act_get_cluster_num_valid"
	k1 := bfrtC.MakeExactKey("hdr.horus.pool_id", uint64(leaf.ID))
	d1 := bfrtC.MakeBytesData("num_ds_elements", uint64(worker_count))
	// Assumption in P4: minimum two spines (one random bit 0|1)
	d2 := bfrtC.MakeBytesData("num_us_elements", uint64(num_spines))
	ks := bfrtC.MakeKeys(k1)
	ds := bfrtC.MakeData(d1, d2)
	err := updateOrInsert(ctx, "Leaf", bfrtclient, table, ks, action, ds)
	if err != nil {
		logrus.Fatal(err.Error())
	}

	// Table entries for port mapping of available upstream spines
	table = "pipe_leaf.LeafIngress.get_spine_dst_id"
	action = "LeafIngress.act_get_spine_dst_id"

	for spine_idx := 0; spine_idx < num_spines; spine_idx++ {
		k1 := bfrtC.MakeExactKey("horus_md.random_id_1", uint64(spine_idx))
		k2 := bfrtC.MakeExactKey("hdr.horus.pool_id", uint64(leaf.ID))
		ks := bfrtC.MakeKeys(k1, k2)
		// Both will have the same spine ID (at index 0) in our testbed (one spine only)
		d1 := bfrtC.MakeBytesData("spine_dst_id", uint64(spines[0].ID))
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
	k1 = bfrtC.MakeExactKey("hdr.horus.pool_id", uint64(leaf.ID))
	d1 = bfrtC.MakeBytesData("cluster_unit", uint64(qlen_unit))
	ks = bfrtC.MakeKeys(k1)
	ds = bfrtC.MakeData(d1)
	err = updateOrInsert(ctx, "Leaf", bfrtclient, table, ks, action, ds)
	if err != nil {
		logrus.Fatal(err.Error())
	}

	reg = "pipe_leaf.LeafIngress.linked_iq_sched"
	rentry = bfrtclient.NewRegisterEntry(reg, uint64(leaf.ID), uint64(spine.ID), nil)
	if err := bfrtclient.InsertTableEntry(ctx, rentry); err != nil {
		logrus.Fatalf("[Leaf] Setting up register %s failed", reg)
	}
	reg = "pipe_leaf.LeafIngress.linked_sq_sched"
	rentry = bfrtclient.NewRegisterEntry(reg, uint64(leaf.ID), uint64(spine.ID), nil)
	if err := bfrtclient.InsertTableEntry(ctx, rentry); err != nil {
		logrus.Fatalf("[Leaf] Setting up register %s failed", reg)
	}
}

func OnServerChangeAllVer(ctx context.Context, leaf *model.Node, updated []*model.Node, bfrtclient *bfrtC.Client, added bool) {
	logrus.Info("Initializing Leaf CP common for all Versions")

	// findout number of alive workers
	worker_count := uint16(0)
	for _, server := range leaf.Children {
		worker_count += server.LastWorkerID - server.FirstWorkerID + 1
	}

	// Update entires #available workers
	table := "pipe_leaf.LeafIngress.get_cluster_num_valid"
	action := "LeafIngress.act_get_cluster_num_valid"
	// Parham: Assumed e.ctrlID is 0-indexed and indicates virtual leaf ID?
	k1 := bfrtC.MakeExactKey("hdr.horus.pool_id", uint64(leaf.ID))
	d1 := bfrtC.MakeBytesData("num_ds_elements", uint64(worker_count))
	// Parham: How can we access number of spines avilable (random linkage for idle count),
	d2 := bfrtC.MakeBytesData("num_us_elements", uint64(2)) // put constant here works in our testbed but should be modified
	ks := bfrtC.MakeKeys(k1)
	ds := bfrtC.MakeData(d1, d2)
	err := updateOrInsert(ctx, "Leaf", bfrtclient, table, ks, action, ds)
	if err != nil {
		logrus.Fatal(err.Error())
	}

	// Update qlen unit
	qlen_unit, _ := model.WorkerQlenUnitMap[worker_count]
	table = "pipe_leaf.LeafIngress.set_queue_len_unit"
	action = "LeafIngress.act_set_queue_len_unit"
	k1 = bfrtC.MakeExactKey("hdr.horus.pool_id", uint64(leaf.ID))
	d1 = bfrtC.MakeBytesData("cluster_unit", uint64(qlen_unit))
	ks = bfrtC.MakeKeys(k1)
	ds = bfrtC.MakeData(d1)
	err = updateOrInsert(ctx, "Leaf", bfrtclient, table, ks, action, ds)
	if err != nil {
		logrus.Fatal(err.Error())
	}

	// Update server port mappings
	table = "pipe_leaf.LeafIngress.forward_horus_switch_dst"
	action = "LeafIngress.act_forward_horus"
	for _, server := range updated {
		for wid := server.FirstWorkerID; wid <= server.LastWorkerID; wid++ {
			// Parham: Assumed e.ctrlID is 0-indexed and indicates virtual leaf ID?
			index := uint16(leaf.ID)*model.MAX_VCLUSTER_WORKERS + wid
			// Table entries for worker index to port mappings
			hw, err := net.ParseMAC(server.Address)
			hw = append(make([]byte, 8-len(hw)), hw...)
			mac_data := binary.BigEndian.Uint64(hw)
			if err != nil {
				logrus.Fatal(err)
			}
			k1 := bfrtC.MakeExactKey("hdr.horus.dst_id", uint64(index))
			k2 := bfrtC.MakeExactKey("hdr.horus.pool_id", uint64(leaf.ID))
			ks := bfrtC.MakeKeys(k1, k2)
			// d1 := bfrtC.MakeBytesData("port", uint64(server.PortId))
			d1 := bfrtC.MakeBytesData("port", uint64(server.Port.GetDevPort()))
			d2 := bfrtC.MakeBytesData("dst_mac", mac_data)
			ds := bfrtC.MakeData(d1, d2)
			err = updateOrInsert(ctx, "Leaf", bfrtclient, table, ks, action, ds)
			if err != nil {
				logrus.Fatal(err.Error())
			}
		}
	}

	if added {
		// read current idle count
		regIdleCount := "pipe_leaf.LeafIngress.idle_count"
		idleCount, err1 := bfrtclient.ReadRegister(ctx, regIdleCount, uint64(leaf.ID))
		if err1 != nil {
			logrus.Fatal("Cannot read register")
		}
		// add new workers to idle list
		reg := "pipe_leaf.LeafIngress.idle_list"
		for _, server := range updated {
			for wid := server.FirstWorkerID; wid <= server.LastWorkerID; wid++ {
				index := leaf.ID*model.MAX_VCLUSTER_WORKERS + wid
				rentry := bfrtclient.NewRegisterEntry(reg, uint64(index), uint64(idleCount), nil)
				if err := bfrtclient.InsertTableEntry(ctx, rentry); err != nil {
					logrus.Errorf("[Leaf] Setting up register %s at index = %d failed", reg, index)
					logrus.Fatal(rentry)
				}
				idleCount += 1
			}
		}
		// update idle count
		bfrtclient.NewRegisterEntry(regIdleCount, uint64(leaf.ID), idleCount, nil)
	}
}

func ManagerInitAllVer(ctx context.Context, bfrtclient *bfrtC.Client) {
	logrus.Debugf("[Manager] Setting up common leaf table entries for all emulated leaves")

	// // Table entry for CPU port
	// table := "pipe_leaf.LeafIngress.forward_horus_switch_dst"
	// action := "LeafIngress.act_forward_horus"
	// k1 := bfrtC.MakeExactKey("hdr.horus.dst_id", uint64(model.CPU_PORT_ID))
	// ks := bfrtC.MakeKeys(k1)
	// d1 := bfrtC.MakeBytesData("port", uint64(model.CPU_PORT_ID))
	// d2 := bfrtC.MakeBytesData("dst_mac", uint64(1000)) // Dummy
	// ds := bfrtC.MakeData(d1, d2)
	// entry := bfrtclient.NewTableEntry(table, ks, action, ds, nil)
	// if err := bfrtclient.InsertTableEntry(ctx, entry); err != nil {
	// 	logrus.Fatal(entry)
	// }

}

func SpineCPInitAllVer(ctx context.Context,
	bfrtclient *bfrtC.Client,
	spine *model.Node,
	topology *model.Topology) {
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
		table := "pipe_spine.SpineIngress.forward_horus_switch_dst"
		action := "SpineIngress.act_forward_horus"
		k1 := bfrtC.MakeExactKey("hdr.horus.dst_id", uint64(leaf.ID))
		// Khaled: Check PortId -> DsPort.GetDevPort()
		// d1 := bfrtC.MakeBytesData("port", uint64(leaf.PortId))
		d1 := bfrtC.MakeBytesData("port", uint64(leaf.DsPort.GetDevPort()))
		ks := bfrtC.MakeKeys(k1)
		ds := bfrtC.MakeData(d1)
		entry := bfrtclient.NewTableEntry(table, ks, action, ds, nil)
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

	// Client port mapping
	for _, client := range topology.Clients.Internal() {
		key := client.ID
		value := client.Port.GetDevPort()
		table := "pipe_spine.SpineIngress.forward_horus_switch_dst"
		action := "SpineIngress.act_forward_horus"
		k1 := bfrtC.MakeExactKey("hdr.horus.dst_id", uint64(key))
		d1 := bfrtC.MakeBytesData("port", uint64(value))
		ks := bfrtC.MakeKeys(k1)
		ds := bfrtC.MakeData(d1)
		entry := bfrtclient.NewTableEntry(table, ks, action, ds, nil)
		if err := bfrtclient.InsertTableEntry(ctx, entry); err != nil {
			logrus.Errorf("[Spine] Setting up table %s failed", table)
			logrus.Fatal(entry)
		}
	}

}

func SpineCPOnLeafChangeAllVer(ctx context.Context,
	bfrtclient *bfrtC.Client,
	spine *model.Node,
	leafID uint64,
	topology *model.Topology,
	added bool) {

	//vcID := uint64(0)
	//idleCount := uint64(0)

	// regIdleCount := "pipe_spine.SpineIngress.idle_count"
	// val, err1 := bfrtclient.ReadRegister(ctx, regIdleCount, vcID)
	// idleCount = val
	// if err1 != nil {
	// 	logrus.Fatal("Cannot read register")
	// }

	// regIdleIdxMap := "pipe_spine.SpineIngress.idle_list_idx_mapping"
	// indexAtIdleList, err1 := bfrtclient.ReadRegister(ctx, regIdleIdxMap, leafID)
	// if err1 != nil {
	// 	logrus.Fatal("Cannot read register")
	// }
	if added {
		// regIdleList := "pipe_spine.SpineIngress.idle_list"
		// rentry := bfrtclient.NewRegisterEntry(regIdleList, idleCount, leafID, nil) // write last element on new index
		// if err := bfrtclient.InsertTableEntry(ctx, rentry); err != nil {
		// 	logrus.Fatalf("[SpineBus] Writing on register %s failed", regIdleList)
		// }
		// rentry = bfrtclient.NewRegisterEntry(regIdleIdxMap, leafID, idleCount, nil) // update index map
		// if err := bfrtclient.InsertTableEntry(ctx, rentry); err != nil {
		// 	logrus.Fatalf("[SpineBus] Writing on register %s failed", regIdleIdxMap)
		// }
		// idleCount += 1
		// rentry = bfrtclient.NewRegisterEntry(regIdleCount, vcID, idleCount, nil)
		// if err := bfrtclient.InsertTableEntry(ctx, rentry); err != nil {
		// 	logrus.Fatalf("[SpineBus] Writing on register %s failed", regIdleCount)
		// }
		// Add Leaf port mapping
		leaf := topology.GetNode(uint16(leafID), model.NodeType_Leaf)
		table := "pipe_spine.SpineIngress.forward_horus_switch_dst"
		action := "SpineIngress.act_forward_horus"
		k1 := bfrtC.MakeExactKey("hdr.horus.dst_id", uint64(leafID))
		// Khaled: Check PortId -> DsPort.GetDevPort()
		d1 := bfrtC.MakeBytesData("port", uint64(leaf.DsPort.GetDevPort()))
		ks := bfrtC.MakeKeys(k1)
		ds := bfrtC.MakeData(d1)
		err := updateOrInsert(ctx, "Spine", bfrtclient, table, ks, action, ds)
		if err != nil {
			logrus.Fatal(err.Error())
		}
	} 
	// else {
	// 	if indexAtIdleList != INVALID_VAL_16bit { // Failed leaf was in the idle list of spine
	// 		logrus.Debugf("[SpineBus] failed leaf was in idle list at index %d", indexAtIdleList)
	// 		rentry := bfrtclient.NewRegisterEntry(regIdleIdxMap, leafID, INVALID_VAL_16bit, nil) // write invalid val on the mapping reg
	// 		if err := bfrtclient.InsertTableEntry(ctx, rentry); err != nil {
	// 			logrus.Fatalf("[SpineBus] Writing on register %s failed", regIdleIdxMap)
	// 		}
	// 		if indexAtIdleList < idleCount-1 { // Read Idle list and swap write the last element on the index of the recently failed leaf
	// 			if idleCount > 1 { // Otherwise, no need to swap, decrementing the idle count will do the job!
	// 				// Read last idle list element
	// 				regIdleList := "pipe_spine.SpineIngress.idle_list"
	// 				lastElement, _ := bfrtclient.ReadRegister(ctx, regIdleList, idleCount-1)
	// 				rentry = bfrtclient.NewRegisterEntry(regIdleList, indexAtIdleList, lastElement, nil) // write last element on new index
	// 				if err := bfrtclient.InsertTableEntry(ctx, rentry); err != nil {
	// 					logrus.Fatalf("[SpineBus] Writing on register %s failed", regIdleList)
	// 				}
	// 				rentry = bfrtclient.NewRegisterEntry(regIdleIdxMap, lastElement, indexAtIdleList, nil) // update index map
	// 				if err := bfrtclient.InsertTableEntry(ctx, rentry); err != nil {
	// 					logrus.Fatalf("[SpineBus] Writing on register %s failed", regIdleIdxMap)
	// 				}
	// 			}
	// 		}
	// 		// Decrement idle count
	// 		rentry = bfrtclient.NewRegisterEntry(regIdleCount, vcID, uint64(math.Max(0, float64(idleCount-1))), nil)
	// 		if err := bfrtclient.InsertTableEntry(ctx, rentry); err != nil {
	// 			logrus.Fatalf("[SpineBus] Writing on register %s failed", regIdleCount)
	// 		}
	// 	}
	// }
}

type ManagerCP interface {
	Init()
	InitRandomAdjustTables()
	MonitorStats(context.Context)
	DumpFinalStats(context.Context)
}

type LeafCP interface {
	Init()
	OnServerChange([]*model.Node, bool)
	Cleanup(uint16)
}

type SpineCP interface {
	Init()
	InitRandomAdjustTables()
	OnLeafChange(uint64, uint64, bool)
	MonitorStats()
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
func (cp *FakeManagerCP) MonitorStats(context.Context) {
	logrus.Info("Calling FakeManagerCP.MonitorStats")	
}

func (cp *FakeManagerCP) DumpFinalStats(context.Context) {
	logrus.Info("Calling FakeManagerCP.DumpFinalStats")	
}

func (cp *BfrtManagerCP_V2) MonitorStats(context.Context) {
	logrus.Info("Calling BfrtManagerCP_V2.MonitorStats")	
}

func (cp *BfrtManagerCP_V2) DumpFinalStats(context.Context) {
	logrus.Info("Calling BfrtManagerCP_V2.DumpFinalStats")	
}

type FakeLeafCP struct {
	leaf     *model.Node
	Topology *model.Topology
	spines   []_SpineConfig
}

func NewFakeLeafCP(leaf *model.Node,
	spines []_SpineConfig,
	topology *model.Topology) *FakeLeafCP {
	return &FakeLeafCP{
		leaf:     leaf,
		spines:   spines,
		Topology: topology,
	}
}

func (cp *FakeLeafCP) Init() {
	logrus.Infof("Initializing FakeLeafCP for Leaf %d", cp.leaf.ID)
}
func (cp *FakeLeafCP) OnServerChange([]*model.Node, bool) {
	logrus.Infof("Calling FakeLeafCP.OnServerChange for Leaf %d", cp.leaf.ID)
}

type FakeSpineCP struct {
	spine    *model.Node
	Topology *model.Topology
}

func NewFakeSpineCP(spine *model.Node, topology *model.Topology) *FakeSpineCP {
	return &FakeSpineCP{
		spine:    spine,
		Topology: topology,
	}
}
func (cp *FakeSpineCP) Init() {
	logrus.Info("Initializing FakeLeafCP")
}

func (cp *FakeSpineCP) InitRandomAdjustTables() {
	logrus.Info("Calling FakeSpineCP.InitRandomAdjustTables")
}
func (cp *FakeSpineCP) OnLeafChange(uint64, uint64, bool) {
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

	ctx := context.Background()
	bfrtclient := cp.client
	ManagerInitAllVer(ctx, bfrtclient)
	cp.InitRandomAdjustTables()
}


type StatsObject struct {
	TotalTaskCount uint64
	TotalResubLeaf uint64
	TotalMsgIdle uint64
	TotalMsgLoad uint64
	TotalStateUpdateMessages uint64
}

func (cp *BfrtManagerCP_V1) fetchStats(ctx context.Context) StatsObject{
	statsObject := StatsObject{}

	bfrtclient := cp.client
    regResubCount := "pipe_leaf.LeafIngress.stat_count_resub"
    regLoadSignalCount := "pipe_leaf.LeafIngress.stat_count_load_signal"
    regIdleSignalCount := "pipe_leaf.LeafIngress.stat_count_idle_signal"
    regTaskCount := "pipe_leaf.LeafIngress.stat_count_task"
    // regAggQlen := "pipe_leaf.LeafIngress.aggregate_queue_len_list"

    // Parham: Just quickly added this, manager has no access to topology, for-loop should be in range of #Emulated Leaves
	for i:=0; i < 4; i++ {
		val, _ := bfrtclient.ReadRegister(ctx, regResubCount, uint64(i))
		statsObject.TotalResubLeaf += val
		
		val, _ = bfrtclient.ReadRegister(ctx, regLoadSignalCount, uint64(i))
		statsObject.TotalMsgLoad += val
		
		val, _ = bfrtclient.ReadRegister(ctx, regIdleSignalCount, uint64(i))
		statsObject.TotalMsgIdle += val
		logrus.Infof("Idle messages[%d]: %d", i, val)
	}

	statsObject.TotalTaskCount, _ = bfrtclient.ReadRegister(ctx, regTaskCount, 0)
	statsObject.TotalStateUpdateMessages = statsObject.TotalMsgIdle + statsObject.TotalMsgLoad

	return statsObject
}


func (cp *BfrtManagerCP_V1) MonitorStats(ctx context.Context) {	
	statsObject := cp.fetchStats(ctx)
	logrus.Infof("MonitorStats::Total tasks arrived at Leaf: %d", statsObject.TotalTaskCount)
	logrus.Infof("MonitorStats::Leaf Total Resubmission: %d", statsObject.TotalResubLeaf)
	logrus.Infof("MonitorStats::Total Msgs for Load Signals: %d", statsObject.TotalMsgLoad)
	logrus.Infof("MonitorStats::Total Msgs for Idle Signals: %d", statsObject.TotalMsgIdle)
	logrus.Infof("MonitorStats::Total State Update Msgs: %d", statsObject.TotalStateUpdateMessages)
}

func (cp *BfrtManagerCP_V1) DumpFinalStats(_ context.Context) {
	statsObject := cp.fetchStats(context.Background())
	logrus.Debug("Starting Gracefull Shutdown")
	jsonFormatted, _ := json.MarshalIndent(statsObject, "", "    ")
	fmt.Println("json formatted: ", string(jsonFormatted))

	statsFileName := fmt.Sprintf("manager-stats-%d.json", time.Now().Unix())

	err := ioutil.WriteFile(statsFileName, jsonFormatted, 0644)
	if err != nil {
		logrus.Debug("Error writing stats to file:", err)  //print the failed message
		return
	}

	logrus.Debugf("Successfully dumped to the file %s.", statsFileName)  //print the success message
}


func (cp *BfrtManagerCP_V1) InitRandomAdjustTables() {
	logrus.Info("Calling BfrtManagerCP.InitRandomAdjustTables")
	bfrtclient := cp.client
	ctx := context.Background()
	for i := 1; i <= 5; i++ {
		table_ds := "pipe_leaf.LeafIngress.adjust_random_range_ds"
		action := fmt.Sprintf("LeafIngress.adjust_random_worker_range_%d", i)
		k_ds_1 := bfrtC.MakeExactKey("horus_md.cluster_num_valid_ds", uint64(math.Pow(2, float64(i))))
		k_ds := bfrtC.MakeKeys(k_ds_1)
		entry_ds := bfrtclient.NewTableEntry(table_ds, k_ds, action, nil, nil) // Parham: works with nil data?
		if err := bfrtclient.InsertTableEntry(ctx, entry_ds); err != nil {
			logrus.Fatal(entry_ds)
		}
		table_us := "pipe_leaf.LeafIngress.adjust_random_range_us"
		k_us_1 := bfrtC.MakeExactKey("horus_md.cluster_num_valid_us", uint64(math.Pow(2, float64(i))))
		k_us := bfrtC.MakeKeys(k_us_1)
		entry_us := bfrtclient.NewTableEntry(table_us, k_us, action, nil, nil) // Parham: works with nil data?
		if err := bfrtclient.InsertTableEntry(ctx, entry_us); err != nil {
			logrus.Fatal(entry_us)
		}
	}
}

type BfrtLeafCP_V1 struct {
	client   *bfrtC.Client
	leaf     *model.Node
	Topology *model.Topology
	spines   []_SpineConfig
}

func NewBfrtLeafCP_V1(client *bfrtC.Client, leaf *model.Node, spines []_SpineConfig,
	topology *model.Topology) *BfrtLeafCP_V1 {
	return &BfrtLeafCP_V1{
		client:   client,
		leaf:     leaf,
		spines:   spines,
		Topology: topology,
	}
}

func (cp *BfrtLeafCP_V1) Init() {
	logrus.Info("Initializing BfrtLeafCP_V1")
	ctx := context.Background()

	if cp.leaf == nil {
		logrus.Fatalf("[Leaf] Leaf doesn't exist")
	}

	LeafCPInitAllVer(ctx, cp.leaf, cp.client, cp.spines, cp.Topology)
}

/* 
 * Parham: This function is only needed when multiple leaves are *emulated* using one switch. 
 * The failed switch state should be cleaned up and same memory location is reused by the next added leaf.
 * Cleanup() Resets the linkeage state and aggregate queue len state of the *failed* leaf.
*/
func (cp *BfrtLeafCP_V1) Cleanup(leafID uint16) {
	bfrtclient := cp.client
	ctx := context.Background()
	reg := "pipe_leaf.LeafIngress.linked_iq_sched"
	rentry := bfrtclient.NewRegisterEntry(reg, uint64(leafID), uint64(INVALID_VAL_16bit), nil)
	if err := bfrtclient.InsertTableEntry(ctx, rentry); err != nil {
		logrus.Fatalf("[Leaf] Setting up register %s failed", reg)
	}
	reg = "pipe_leaf.LeafIngress.linked_sq_sched"
	rentry = bfrtclient.NewRegisterEntry(reg, uint64(leafID), uint64(INVALID_VAL_16bit), nil)
	if err := bfrtclient.InsertTableEntry(ctx, rentry); err != nil {
		logrus.Fatalf("[Leaf] Setting up register %s failed", reg)
	}
	reg = "pipe_leaf.LeafIngress.aggregate_queue_len_list"
	rentry = bfrtclient.NewRegisterEntry(reg, uint64(leafID), uint64(0), nil)
	if err := bfrtclient.InsertTableEntry(ctx, rentry); err != nil {
		logrus.Fatalf("[Leaf] Setting up register %s failed", reg)
	}
}

func (cp *BfrtLeafCP_V2) Cleanup(leafID uint16) {
	bfrtclient := cp.client
	ctx := context.Background()
	reg := "pipe_leaf.LeafIngress.linked_iq_sched"
	rentry := bfrtclient.NewRegisterEntry(reg, uint64(leafID), uint64(INVALID_VAL_16bit), nil)
	if err := bfrtclient.InsertTableEntry(ctx, rentry); err != nil {
		logrus.Fatalf("[Leaf] Setting up register %s failed", reg)
	}
	reg = "pipe_leaf.LeafIngress.linked_sq_sched"
	rentry = bfrtclient.NewRegisterEntry(reg, uint64(leafID), uint64(INVALID_VAL_16bit), nil)
	if err := bfrtclient.InsertTableEntry(ctx, rentry); err != nil {
		logrus.Fatalf("[Leaf] Setting up register %s failed", reg)
	}
	reg = "pipe_leaf.LeafIngress.aggregate_queue_len_list"
	rentry = bfrtclient.NewRegisterEntry(reg, uint64(leafID), uint64(0), nil)
	if err := bfrtclient.InsertTableEntry(ctx, rentry); err != nil {
		logrus.Fatalf("[Leaf] Setting up register %s failed", reg)
	}
}

func (cp *FakeLeafCP) Cleanup(leafID uint16) {
	logrus.Info("Fake leaf cleanup")
}

func (cp *BfrtLeafCP_V1) OnServerChange(updated []*model.Node, added bool) {
	logrus.Debugf("[LeafBus-%d] Updating tables after server changes", cp.leaf.ID)

	ctx := context.Background()
	bfrtclient := cp.client
	OnServerChangeAllVer(ctx, cp.leaf, updated, bfrtclient, added)

}

type BfrtSpineCP_V1 struct {
	client   *bfrtC.Client
	spine    *model.Node
	Topology *model.Topology
}

func NewBfrtSpineCP_V1(client *bfrtC.Client,
	spine *model.Node,
	topology *model.Topology) *BfrtSpineCP_V1 {
	return &BfrtSpineCP_V1{
		client:   client,
		spine:    spine,
		Topology: topology,
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

	SpineCPInitAllVer(ctx, bfrtclient, spine, cp.Topology)

	// Parham: assuming single VC for now TODO: check and fix later
	var vcID uint64 = 0
	for i, leaf := range spine.Children {
		worker_count := uint16(0)
		for _, server := range leaf.Children {
			// Parham: Check this line seems redundant, how can we access the worker count of server
			worker_count += server.LastWorkerID - server.FirstWorkerID + 1
		}
		// LeafId -> qlen_unit mapping (two tables since we have two samples)
		table := "pipe_spine.SpineIngress.set_queue_len_unit_1"
		action := "SpineIngress.act_set_queue_len_unit_1"
		k1 := bfrtC.MakeExactKey("horus_md.random_id_1", uint64(leaf.ID))
		k2 := bfrtC.MakeExactKey("hdr.horus.pool_id", vcID)
		d1 := bfrtC.MakeBytesData("cluster_unit", uint64(model.WorkerQlenUnitMap[worker_count]))
		ks := bfrtC.MakeKeys(k1, k2)
		ds := bfrtC.MakeData(d1)
		entry := bfrtclient.NewTableEntry(table, ks, action, ds, nil)
		if err := bfrtclient.InsertTableEntry(ctx, entry); err != nil {
			logrus.Errorf("[Spine] Setting up table %s failed", table)
			logrus.Fatal(entry)
		}

		table = "pipe_spine.SpineIngress.set_queue_len_unit_2"
		action = "SpineIngress.act_set_queue_len_unit_2"
		k1 = bfrtC.MakeExactKey("horus_md.random_id_2", uint64(leaf.ID))
		k2 = bfrtC.MakeExactKey("hdr.horus.pool_id", vcID)
		d1 = bfrtC.MakeBytesData("cluster_unit", uint64(model.WorkerQlenUnitMap[worker_count]))
		ks = bfrtC.MakeKeys(k1, k2)
		ds = bfrtC.MakeData(d1)
		entry = bfrtclient.NewTableEntry(table, ks, action, ds, nil)
		if err := bfrtclient.InsertTableEntry(ctx, entry); err != nil {
			logrus.Errorf("[Spine] Setting up table %s failed", table)
			logrus.Fatal(entry)
		}

		table = "pipe_spine.SpineIngress.set_queue_len_unit_resub"
		action = "SpineIngress.act_set_queue_len_unit_resub"
		k1 = bfrtC.MakeExactKey("horus_md.selected_ds_index", uint64(leaf.ID))
		k2 = bfrtC.MakeExactKey("hdr.horus.pool_id", vcID)
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
		k1 = bfrtC.MakeExactKey("horus_md.random_ds_index_1", uint64(i))
		k2 = bfrtC.MakeExactKey("hdr.horus.pool_id", vcID)
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
		k1 = bfrtC.MakeExactKey("horus_md.random_ds_index_2", uint64(i))
		k2 = bfrtC.MakeExactKey("hdr.horus.pool_id", vcID)
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
		k1 = bfrtC.MakeExactKey("hdr.horus.pool_id", vcID)
		k2 = bfrtC.MakeExactKey("hdr.horus.src_id", uint64(leaf.ID))
		d1 = bfrtC.MakeBytesData("switch_index", uint64(i))
		ks = bfrtC.MakeKeys(k1, k2)
		ds = bfrtC.MakeData(d1)
		entry = bfrtclient.NewTableEntry(table, ks, action, ds, nil)
		if err := bfrtclient.InsertTableEntry(ctx, entry); err != nil {
			logrus.Errorf("[Spine] Setting up table %s failed", table)
			logrus.Fatal(entry)
		}
	}

	// vcID -> #leaves mapping
	table := "pipe_spine.SpineIngress.get_cluster_num_valid_leafs"
	action := "SpineIngress.act_get_cluster_num_valid_leafs"
	k1 := bfrtC.MakeExactKey("hdr.horus.pool_id", vcID)
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
	for i := 2; i <= 32; i++ {
		table_ds := "pipe_spine.SpineIngress.adjust_random_range_sq_leafs"
		num_bits := uint64(math.Ceil(math.Log2(float64(i))))
		action := fmt.Sprintf("SpineIngress.adjust_random_leaf_index_%d", num_bits)
		k_ds_1 := bfrtC.MakeExactKey("horus_md.cluster_num_valid_queue_signals", uint64(i))
		k_ds := bfrtC.MakeKeys(k_ds_1)
		logrus.Debugf("bits=%d, key=%d, action=%s", num_bits, i, action)
		entry_ds := bfrtclient.NewTableEntry(table_ds, k_ds, action, nil, nil) // Parham: works with nil data?
		if err := bfrtclient.InsertTableEntry(ctx, entry_ds); err != nil {
			logrus.Error(err.Error())
			logrus.Fatal(entry_ds)
		}
	}
}

func (cp *BfrtSpineCP_V1) OnLeafChange(leafID uint64, index uint64, added bool) {
	logrus.Info("Calling BfrtSpineCP_V1.OnLeafChange")
	logrus.Debugf("[SpineBus-%d] Updating tables after for leaf %d at index %d", cp.spine.ID, leafID, index)
	spine := cp.spine
	ctx := context.Background()
	bfrtclient := cp.client
	vcID := uint64(0)

	SpineCPOnLeafChangeAllVer(ctx, bfrtclient, spine, leafID, cp.Topology, added)
	childCount := len(spine.Children)

	// Parham: Assuming leaf IDs stay the same after failure but leaf indices were updated accordingly
	for _, leaf := range spine.Children {
		// Mapping leafID -> index (of qlen list)
		table := "pipe_spine.SpineIngress.get_switch_index"
		action := "SpineIngress.act_get_switch_index"
		k1 := bfrtC.MakeExactKey("hdr.horus.pool_id", vcID)
		k2 := bfrtC.MakeExactKey("hdr.horus.src_id", uint64(leaf.ID))
		d1 := bfrtC.MakeBytesData("switch_index", uint64(leaf.Index))
		ks := bfrtC.MakeKeys(k1, k2)
		ds := bfrtC.MakeData(d1)
		err := updateOrInsert(ctx, "Spine", bfrtclient, table, ks, action, ds)
		if err != nil {
			logrus.Fatal(err.Error())
		}
		// index (of qlen list) -> leafID mapping
		table = "pipe_spine.SpineIngress.get_rand_leaf_id_1"
		action = "SpineIngress.act_get_rand_leaf_id_1"
		k1 = bfrtC.MakeExactKey("horus_md.random_ds_index_1", uint64(leaf.Index))
		k2 = bfrtC.MakeExactKey("hdr.horus.pool_id", vcID)
		d1 = bfrtC.MakeBytesData("leaf_id", uint64(leaf.ID))
		ks = bfrtC.MakeKeys(k1, k2)
		ds = bfrtC.MakeData(d1)
		err = updateOrInsert(ctx, "Spine", bfrtclient, table, ks, action, ds)
		if err != nil {
			logrus.Fatal(err.Error())
		}

		table = "pipe_spine.SpineIngress.get_rand_leaf_id_2"
		action = "SpineIngress.act_get_rand_leaf_id_2"
		k1 = bfrtC.MakeExactKey("horus_md.random_ds_index_2", uint64(leaf.Index))
		k2 = bfrtC.MakeExactKey("hdr.horus.pool_id", vcID)
		d1 = bfrtC.MakeBytesData("leaf_id", uint64(leaf.ID))
		ks = bfrtC.MakeKeys(k1, k2)
		ds = bfrtC.MakeData(d1)
		err = updateOrInsert(ctx, "Spine", bfrtclient, table, ks, action, ds)
		if err != nil {
			logrus.Fatal(err.Error())
		}
	}

	// Decrement total number of available children
	table := "pipe_spine.SpineIngress.get_cluster_num_valid_leafs"
	action := "SpineIngress.act_get_cluster_num_valid_leafs"
	k1 := bfrtC.MakeExactKey("hdr.horus.pool_id", vcID)
	d1 := bfrtC.MakeBytesData("num_leafs", uint64(childCount))
	ks := bfrtC.MakeKeys(k1)
	ds := bfrtC.MakeData(d1)
	err := updateOrInsert(ctx, "Spine", bfrtclient, table, ks, action, ds)
	if err != nil {
		logrus.Fatal(err.Error())
	}

	// Copy and shift to left cell the queue len lists for every index i where i < falied leaf index
	// Parham: We need to check since these are being modified by data plane it might be already updated by leaf and this move
	// results in incorrect state until next queue signal arrives from the leaf
	qlenList1 := "pipe_spine.SpineIngress.queue_len_list_1"
	qlenList2 := "pipe_spine.SpineIngress.queue_len_list_2"
	defList1 := "pipe_spine.SpineIngress.deferred_queue_len_list_1"
	defList2 := "pipe_spine.SpineIngress.deferred_queue_len_list_2"
	// for i := int(index) + 1; i <= len(spine.Children); i++ {

	// 	nextCellQlen, _ := bfrtclient.ReadRegister(ctx, qlenList1, uint64(i))
	// 	nextCellDrift, _ := bfrtclient.ReadRegister(ctx, defList1, uint64(i))

	// 	rentry := bfrtclient.NewRegisterEntry(qlenList1, uint64(i-1), nextCellQlen+nextCellDrift, nil)
	// 	if err := bfrtclient.InsertTableEntry(ctx, rentry); err != nil {
	// 		logrus.Fatalf("[SpineBus] Writing on register %s failed", qlenList1)
	// 	}
	// 	rentry = bfrtclient.NewRegisterEntry(qlenList2, uint64(i-1), nextCellQlen+nextCellDrift, nil)
	// 	if err := bfrtclient.InsertTableEntry(ctx, rentry); err != nil {
	// 		logrus.Fatalf("[SpineBus] Writing on register %s failed", qlenList2)
	// 	}

	// 	// reset drift
	// 	rentry = bfrtclient.NewRegisterEntry(defList1, uint64(i-1), 0, nil)
	// 	if err := bfrtclient.InsertTableEntry(ctx, rentry); err != nil {
	// 		logrus.Fatalf("[SpineBus] Writing on register %s failed", defList1)
	// 	}
	// 	rentry = bfrtclient.NewRegisterEntry(defList2, uint64(i-1), 0, nil)
	// 	if err := bfrtclient.InsertTableEntry(ctx, rentry); err != nil {
	// 		logrus.Fatalf("[SpineBus] Writing on register %s failed", defList2)
	// 	}
	// }

	// new slot qlen
	
		rentry := bfrtclient.NewRegisterEntry(qlenList1, uint64(len(spine.Children)-1), uint64(0), nil)
		if err := bfrtclient.InsertTableEntry(ctx, rentry); err != nil {
			logrus.Fatalf("[SpineBus] Writing on register %s failed", qlenList1)
		}
		rentry = bfrtclient.NewRegisterEntry(qlenList2, uint64(len(spine.Children)-1), uint64(0), nil)
		if err := bfrtclient.InsertTableEntry(ctx, rentry); err != nil {
			logrus.Fatalf("[SpineBus] Writing on register %s failed", qlenList1)
		}
		rentry = bfrtclient.NewRegisterEntry(defList1, uint64(len(spine.Children)-1), uint64(0), nil)
		if err := bfrtclient.InsertTableEntry(ctx, rentry); err != nil {
			logrus.Fatalf("[SpineBus] Writing on register %s failed", qlenList1)
		}
		rentry = bfrtclient.NewRegisterEntry(defList2, uint64(len(spine.Children)-1), uint64(0), nil)
		if err := bfrtclient.InsertTableEntry(ctx, rentry); err != nil {
			logrus.Fatalf("[SpineBus] Writing on register %s failed", qlenList1)
		}

	

	
}

type BfrtSpineCP_V2 struct {
	client   *bfrtC.Client
	spine    *model.Node
	Topology *model.Topology
}

func NewBfrtSpineCP_V2(client *bfrtC.Client,
	spine *model.Node,
	topology *model.Topology) *BfrtSpineCP_V2 {
	return &BfrtSpineCP_V2{
		client:   client,
		spine:    spine,
		Topology: topology,
	}
}

func (cp *BfrtSpineCP_V2) Init() {
	logrus.Info("Initializing BfrtSpineCP_V1")
	ctx := context.Background()
	bfrtclient := cp.client
	// Parham: Is controller ID 0 indexed?
	spine := cp.spine
	if spine == nil {
		logrus.Fatalf("[Spine] Spine doesn't exist")
	}

	SpineCPInitAllVer(ctx, bfrtclient, spine, cp.Topology)

	var vcID uint64 = 0

	for _, leaf := range spine.Children {
		worker_count := uint16(0)
		for _, server := range leaf.Children {
			// Parham: Check this line seems redundant, how can we access the worker count of server
			worker_count += server.LastWorkerID - server.FirstWorkerID + 1
		}
		// LeafId -> qlen_unit mapping
		table := "pipe_spine.SpineIngress.set_queue_len_unit_1"
		action := "SpineIngress.act_set_queue_len_unit_1"
		k1 := bfrtC.MakeExactKey("horus_md.low_ds_id", uint64(leaf.ID))
		k2 := bfrtC.MakeExactKey("hdr.horus.pool_id", vcID)
		d1 := bfrtC.MakeBytesData("cluster_unit", uint64(model.WorkerQlenUnitMap[worker_count]))
		ks := bfrtC.MakeKeys(k1, k2)
		ds := bfrtC.MakeData(d1)
		entry := bfrtclient.NewTableEntry(table, ks, action, ds, nil)
		if err := bfrtclient.InsertTableEntry(ctx, entry); err != nil {
			logrus.Errorf("[Spine] Setting up table %s failed", table)
			logrus.Fatal(entry)
		}
	}
	// Init randomly the worker id_map_x: stores the worker ID of the Xth (minimum) qlen.
	// id_map_1 worker ID is minimum id_map_2 is second best.
	reg := "pipe_spine.SpineIngress.leaf_id_map_1"
	rentry := bfrtclient.NewRegisterEntry(reg, uint64(vcID), uint64(spine.Children[0].ID), nil)
	if err := bfrtclient.InsertTableEntry(ctx, rentry); err != nil {
		logrus.Fatalf("[Leaf] Setting up register %s failed", reg)
	}
	reg = "pipe_spine.SpineIngress.leaf_id_map_2"
	rentry = bfrtclient.NewRegisterEntry(reg, uint64(vcID), uint64(spine.Children[len(spine.Children)-1].ID), nil)
	if err := bfrtclient.InsertTableEntry(ctx, rentry); err != nil {
		logrus.Fatalf("[Leaf] Setting up register %s failed", reg)
	}

	// Write qlen = 1 on both low and high cells (cells are only used after there's no idle), minimum qlen is 1
	reg = "pipe_spine.SpineIngress.queue_len_list_low"
	rentry = bfrtclient.NewRegisterEntry(reg, uint64(vcID), uint64(1), nil)
	if err := bfrtclient.InsertTableEntry(ctx, rentry); err != nil {
		logrus.Fatalf("[Leaf] Setting up register %s failed", reg)
	}
	reg = "pipe_spine.SpineIngress.queue_len_list_high"
	rentry = bfrtclient.NewRegisterEntry(reg, uint64(vcID), uint64(1), nil)
	if err := bfrtclient.InsertTableEntry(ctx, rentry); err != nil {
		logrus.Fatalf("[Leaf] Setting up register %s failed", reg)
	}
}

func (cp *BfrtSpineCP_V2) InitRandomAdjustTables() {
	// Intentionally empty: Not used in V2
}

func (cp *BfrtSpineCP_V2) MonitorStats() {
	ctx := context.Background()
	bfrtclient := cp.client
	regIdleCount := "pipe_spine.SpineIngress.idle_count"
	val, err1 := bfrtclient.ReadRegister(ctx, regIdleCount, 0)
	if err1 != nil {
		logrus.Fatal("Cannot read register")
	}
	logrus.Debugf("Idle Count: %d", val)
	for i := 0; i < 4; i++ {
		regIdleList := "pipe_spine.SpineIngress.idle_list"
		idle_element, _ := bfrtclient.ReadRegister(ctx, regIdleList, uint64(i))
		logrus.Debugf("IdleList[%d] = %d: ", i, idle_element)
	}
	qlenList1 := "pipe_spine.SpineIngress.queue_len_list_1"
	qlenList2 := "pipe_spine.SpineIngress.queue_len_list_2"
	defList1 := "pipe_spine.SpineIngress.deferred_queue_len_list_1"
	defList2 := "pipe_spine.SpineIngress.deferred_queue_len_list_2"
	for i := 0; i < 4; i++ {
		val, _ := bfrtclient.ReadRegister(ctx, qlenList1, uint64(i))
		logrus.Debugf("qlenList1[%d] = %d: ", i, val)
		val, _ = bfrtclient.ReadRegister(ctx, qlenList2, uint64(i))
		logrus.Debugf("qlenList2[%d] = %d: ", i, val)
		val, _ = bfrtclient.ReadRegister(ctx, defList1, uint64(i))
		logrus.Debugf("defList1[%d] = %d: ", i, val)
		val, _ = bfrtclient.ReadRegister(ctx, defList2, uint64(i))
		logrus.Debugf("defList2[%d] = %d: ", i, val)
	}
}

func (cp *BfrtSpineCP_V1) MonitorStats() {
	ctx := context.Background()
	bfrtclient := cp.client
	// regIdleCount := "pipe_spine.SpineIngress.idle_count"
	// val, err1 := bfrtclient.ReadRegister(ctx, regIdleCount, 0)
	// if err1 != nil {
	// 	logrus.Fatal("Cannot read register")
	// }
	// logrus.Debugf("Idle Count: %d", val)
	// for i := 0; i < 4; i++ {
	// 	regIdleList := "pipe_spine.SpineIngress.idle_list"
	// 	idle_element, _ := bfrtclient.ReadRegister(ctx, regIdleList, uint64(i))
	// 	logrus.Debugf("IdleList[%d] = %d: ", i, idle_element)
	// }
	// qlenList1 := "pipe_spine.SpineIngress.queue_len_list_1"
	// qlenList2 := "pipe_spine.SpineIngress.queue_len_list_2"
	// defList1 := "pipe_spine.SpineIngress.deferred_queue_len_list_1"
	// defList2 := "pipe_spine.SpineIngress.deferred_queue_len_list_2"
	// for i := 0; i < 4; i++ {
	// 	val, _ := bfrtclient.ReadRegister(ctx, qlenList1, uint64(i))
	// 	logrus.Debugf("qlenList1[%d] = %d: ", i, val)
	// 	val, _ = bfrtclient.ReadRegister(ctx, qlenList2, uint64(i))
	// 	logrus.Debugf("qlenList2[%d] = %d: ", i, val)
	// 	val, _ = bfrtclient.ReadRegister(ctx, defList1, uint64(i))
	// 	logrus.Debugf("defList1[%d] = %d: ", i, val)
	// 	val, _ = bfrtclient.ReadRegister(ctx, defList2, uint64(i))
	// 	logrus.Debugf("defList2[%d] = %d: ", i, val)
	// }

	regTot := "pipe_spine.SpineIngress.stat_count_task"
	regResub := "pipe_spine.SpineIngress.stat_count_resub"
	resubCount, _ := bfrtclient.ReadRegister(ctx, regResub, 0)
	taskCount, _ := bfrtclient.ReadRegister(ctx, regTot, 0)
	logrus.Infof("Total resubmissions at Spine (Task resub + Idle remove resub): %d", resubCount)
	logrus.Infof("Total tasks arrived at Spine: %d", taskCount)
}

func (cp *FakeSpineCP) MonitorStats() {
	// logrus.Debug("Fake monitor switch stats!")
}

func (cp *BfrtSpineCP_V2) OnLeafChange(leafID uint64, index uint64, added bool) {
	logrus.Info("Calling BfrtSpineCP_V2.OnLeafChange")
	logrus.Debugf("[SpineBus-%d] Updating tables after leaf changes", cp.spine.ID)
	spine := cp.spine
	ctx := context.Background()
	bfrtclient := cp.client
	vcID := uint64(0)

	SpineCPOnLeafChangeAllVer(ctx, bfrtclient, spine, leafID, cp.Topology, added)

	regID1 := "pipe_spine.SpineIngress.leaf_id_map_1"
	regID2 := "pipe_spine.SpineIngress.leaf_id_map_2"
	regQlen1 := "pipe_spine.SpineIngress.queue_len_list_low"
	regQlen2 := "pipe_spine.SpineIngress.queue_len_list_high"

	if !added {
		leafID1, _ := bfrtclient.ReadRegister(ctx, regID1, vcID)
		leafID2, _ := bfrtclient.ReadRegister(ctx, regID2, vcID)
		leafhighQlen, _ := bfrtclient.ReadRegister(ctx, regQlen2, vcID)

		if leafID != leafID1 && leafID != leafID2 { // All good! no need to modify the qlen lists
			return
		}

		if leafID1 == leafID { // failed leaf was being tracked as top leaf (min avg qlen)
			// Write ID of previously second-best leaf (which is  alive and now is best child) on first ID reg
			rentry := bfrtclient.NewRegisterEntry(regID1, vcID, leafID2, nil)
			if err := bfrtclient.InsertTableEntry(ctx, rentry); err != nil {
				logrus.Fatalf("[SpineBus] Setting up register %s failed", regID1)
			}
			// Write qlen of previously second-best leaf (which is  alive and now is best child)
			rentry = bfrtclient.NewRegisterEntry(regQlen1, vcID, leafhighQlen, nil)
			if err := bfrtclient.InsertTableEntry(ctx, rentry); err != nil {
				logrus.Fatalf("[SpineBus] Setting up register %s failed", regQlen2)
			}
		}

		/*
			 *  Cases1: workerID2 == leaf ID: Failed leaf was being tracked as second best leaf (2nd min avg qlen),
				OR Case2: failed leaf was best leaf so we 2nd best to that position in the if-statement above
			 * In both case: Should write a large qlen on the second best so it'll be replaced ba any better qlen
		*/
		rentry := bfrtclient.NewRegisterEntry(regQlen2, vcID, uint64(100), nil)
		if err := bfrtclient.InsertTableEntry(ctx, rentry); err != nil {
			logrus.Fatalf("[SpineBus] Setting up register %s failed", regQlen2)
		}
		// Write ID of an (arbitrary) alive leaf so in the case that this one was selected before receiving a new qlen signal,
		// the pkt won't be forwarded to the failed leaf
		rentry = bfrtclient.NewRegisterEntry(regID2, vcID, uint64(spine.Children[0].ID), nil)
		if err := bfrtclient.InsertTableEntry(ctx, rentry); err != nil {
			logrus.Fatalf("[SpineBus] Setting up register %s failed", regID1)
		}
	}
}

type BfrtLeafCP_V2 struct {
	client   *bfrtC.Client
	leaf     *model.Node
	Topology *model.Topology
	spines   []_SpineConfig
}

func NewBfrtLeafCP_V2(client *bfrtC.Client,
	leaf *model.Node,
	spines []_SpineConfig,
	topology *model.Topology) *BfrtLeafCP_V2 {
	return &BfrtLeafCP_V2{
		client:   client,
		leaf:     leaf,
		spines:   spines,
		Topology: topology,
	}
}

func (cp *BfrtLeafCP_V2) Init() {
	logrus.Info("Initializing BfrtLeafCP_V2")
	ctx := context.Background()

	if cp.leaf == nil {
		logrus.Fatalf("[Leaf] Leaf doesn't exist")
	}
	leaf := cp.leaf
	bfrtclient := cp.client

	LeafCPInitAllVer(ctx, cp.leaf, bfrtclient, cp.spines, cp.Topology)

	// Init randomly the worker id_map_x: stores the worker ID of the Xth (minimum) qlen.
	// id_map_1 worker ID is minimum id_map_2 is second best.
	reg := "pipe_leaf.LeafIngress.worker_id_map_1"
	rentry := bfrtclient.NewRegisterEntry(reg, uint64(leaf.Index), uint64(leaf.Children[0].FirstWorkerID), nil)
	if err := bfrtclient.InsertTableEntry(ctx, rentry); err != nil {
		logrus.Fatalf("[Leaf] Setting up register %s failed", reg)
	}
	reg = "pipe_leaf.LeafIngress.worker_id_map_2"
	rentry = bfrtclient.NewRegisterEntry(reg, uint64(leaf.Index), uint64(leaf.Children[0].LastWorkerID), nil)
	if err := bfrtclient.InsertTableEntry(ctx, rentry); err != nil {
		logrus.Fatalf("[Leaf] Setting up register %s failed", reg)
	}

	// Write qlen 1 on both low and high cells (cells are only used after there's no idle), minimum qlen is 1
	reg = "pipe_leaf.LeafIngress.queue_len_list_low"
	rentry = bfrtclient.NewRegisterEntry(reg, uint64(leaf.Index), uint64(1), nil)
	if err := bfrtclient.InsertTableEntry(ctx, rentry); err != nil {
		logrus.Fatalf("[Leaf] Setting up register %s failed", reg)
	}
	reg = "pipe_leaf.LeafIngress.queue_len_list_high"
	rentry = bfrtclient.NewRegisterEntry(reg, uint64(leaf.Index), uint64(1), nil)
	if err := bfrtclient.InsertTableEntry(ctx, rentry); err != nil {
		logrus.Fatalf("[Leaf] Setting up register %s failed", reg)
	}
}

func (cp *BfrtLeafCP_V2) OnServerChange(updated []*model.Node, added bool) {
	logrus.Debugf("[LeafBus-%d] Updating tables after server changes", cp.leaf.ID)

	ctx := context.Background()
	bfrtclient := cp.client
	leaf := cp.leaf

	OnServerChangeAllVer(ctx, leaf, updated, bfrtclient, added)

	regID1 := "pipe_leaf.LeafIngress.worker_id_map_1"
	regID2 := "pipe_leaf.LeafIngress.worker_id_map_2"
	// Parham: Can we have ID of failed workers, to check if current tracked worker IDs were among the failed workers?
	// If failed worker ID was one of the tracked workers, we update its qlen to a large value, otherwse no need for that
	// so it'll be replaced by any other qlen
	//workerID1, _ := bfrtclient.ReadRegister(ctx, regID1, uint64(leaf.Index))
	//workerID2, _ := bfrtclient.ReadRegister(ctx, regID2, uint64(leaf.Index))
	if !added {
		// Write ID of two (arbitrary) alive workers so the new tasks won't be forwarded to failed workers
		rentry := bfrtclient.NewRegisterEntry(regID1, uint64(leaf.ID), uint64(leaf.Children[0].FirstWorkerID), nil)
		if err := bfrtclient.InsertTableEntry(ctx, rentry); err != nil {
			logrus.Fatalf("[LeafBus] Setting up register %s failed", regID1)
		}
		rentry = bfrtclient.NewRegisterEntry(regID2, uint64(leaf.ID), uint64(leaf.Children[len(leaf.Children)-1].LastWorkerID), nil)
		if err := bfrtclient.InsertTableEntry(ctx, rentry); err != nil {
			logrus.Fatalf("[LeafBus] Setting up register %s failed", regID1)
		}

		// Write a large value on the tracked queue lengths so it will be replaced by other workers
		regQlen1 := "pipe_leaf.LeafIngress.queue_len_list_low"
		regQlen2 := "pipe_leaf.LeafIngress.queue_len_list_high"
		rentry = bfrtclient.NewRegisterEntry(regQlen1, uint64(leaf.ID), uint64(100), nil)
		if err := bfrtclient.InsertTableEntry(ctx, rentry); err != nil {
			logrus.Fatalf("[LeafBus] Setting up register %s failed", regQlen1)
		}
		rentry = bfrtclient.NewRegisterEntry(regQlen2, uint64(leaf.ID), uint64(100), nil)
		if err := bfrtclient.InsertTableEntry(ctx, rentry); err != nil {
			logrus.Fatalf("[LeafBus] Setting up register %s failed", regQlen2)
		}
	}
}

type BfrtManagerCP_V2 struct {
	client *bfrtC.Client
}

func NewBfrtManagerCP_V2(client *bfrtC.Client) *BfrtManagerCP_V2 {
	return &BfrtManagerCP_V2{
		client: client,
	}
}

func (cp *BfrtManagerCP_V2) Init() {
	logrus.Info("Initializing BfrtManagerCP_V1")

	ctx := context.Background()
	bfrtclient := cp.client
	ManagerInitAllVer(ctx, bfrtclient)
	cp.InitRandomAdjustTables()
}

func (cp *BfrtManagerCP_V2) InitRandomAdjustTables() {
	logrus.Info("Calling BfrtManagerCP.InitRandomAdjustTables")
	bfrtclient := cp.client
	ctx := context.Background()
	for i := 1; i <= 5; i++ {
		action := fmt.Sprintf("LeafIngress.adjust_random_worker_range_%d", i)
		table_us := "pipe_leaf.LeafIngress.adjust_random_range_us"
		k_us_1 := bfrtC.MakeExactKey("horus_md.cluster_num_valid_us", uint64(math.Pow(2, float64(i))))
		k_us := bfrtC.MakeKeys(k_us_1)
		entry_us := bfrtclient.NewTableEntry(table_us, k_us, action, nil, nil) // Parham: works with nil data?
		if err := bfrtclient.InsertTableEntry(ctx, entry_us); err != nil {
			logrus.Fatal(entry_us)
		}
	}
}
