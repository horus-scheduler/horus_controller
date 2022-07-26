package ctrl_mgr

import (
	"context"

	"github.com/horus-scheduler/horus_controller/core"
	"github.com/horus-scheduler/horus_controller/core/model"
	horus_net "github.com/horus-scheduler/horus_controller/core/net"
	bfrtC "github.com/khaledmdiab/bfrt-go-client/pkg/client"
	"github.com/sirupsen/logrus"
)

// SpineBusChan ...
type SpineBusChan struct {
	// healthManager channels
	hmIngressActiveNode chan *core.LeafHealthMsg // recv-from healthManager
	// send-to healthManager

	// gRPC channels
	rpcFailedLeaves  chan *horus_net.LeafFailedMessage   // recv-from gRPC
	rpcFailedServers chan *horus_net.ServerFailedMessage // recv-from gRPC
	newLeaves        chan *horus_net.LeafAddedMessage
	newServers       chan *horus_net.ServerAddedMessage
	newVCs           chan *horus_net.VCUpdatedMessage

	// ASIC channels
	asicIngress chan []byte // recv-from the ASIC
	asicEgress  chan []byte // send-to the ASIC
}

// NewSpineBusChan ...
func NewSpineBusChan(hmIngressActiveNode chan *core.LeafHealthMsg,
	rpcFailedLeaves chan *horus_net.LeafFailedMessage,
	rpcFailedServers chan *horus_net.ServerFailedMessage,
	newLeaves chan *horus_net.LeafAddedMessage,
	newServers chan *horus_net.ServerAddedMessage,
	newVCs chan *horus_net.VCUpdatedMessage,
	asicIngress chan []byte,
	asicEgress chan []byte) *SpineBusChan {
	return &SpineBusChan{
		hmIngressActiveNode: hmIngressActiveNode,
		rpcFailedLeaves:     rpcFailedLeaves,
		rpcFailedServers:    rpcFailedServers,
		newLeaves:           newLeaves,
		newServers:          newServers,
		newVCs:              newVCs,
		asicIngress:         asicIngress,
		asicEgress:          asicEgress,
	}
}

// SpineBus ...
type SpineBus struct {
	*SpineBusChan
	ctrlID    uint16
	topology  *model.Topology
	vcm       *core.VCManager
	healthMgr *core.LeafHealthManager
	bfrt      *bfrtC.Client // BfRt client
	doneChan  chan bool
}

// NewSpineBus ...
func NewSpineBus(ctrlID uint16,
	busChan *SpineBusChan,
	topology *model.Topology,
	vcm *core.VCManager,
	healthMgr *core.LeafHealthManager,
	bfrt *bfrtC.Client) *SpineBus {
	return &SpineBus{
		SpineBusChan: busChan,
		ctrlID:       ctrlID,
		topology:     topology,
		vcm:          vcm,
		healthMgr:    healthMgr,
		bfrt:         bfrt,
		doneChan:     make(chan bool, 1),
	}
}

func (bus *SpineBus) update_tables_leaf_change(leafID uint64, index uint64) {
	logrus.Debugf("[SpineBus-%d] Updating tables after leaf changes", bus.ctrlID)
	spine := bus.topology.GetNode(bus.ctrlID, model.NodeType_Spine)

	INVALID_VAL_16bit := uint64(0x7FFF)
	ctx := context.Background()
	bfrtclient := bus.bfrt
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

func (bus *SpineBus) processIngress() {
	for {
		select {
		case message := <-bus.rpcFailedLeaves:
			// TODO: receives a msg that a leaf had failed
			// Notice: At this stage, the failed leaf has already been removed and detached
			go func() {
				logrus.Debugf("[SpineBus-%d] Using BfRt Client to remove leaf-related DP info from spine; leafID = %d, leafIndex= %d",
					bus.ctrlID, message.Leaf.Id, message.Leaf.Index)
				if bus.topology != nil {
					bus.topology.Debug()
				}
				bus.update_tables_leaf_change(uint64(message.Leaf.Id), uint64(message.Leaf.Index))
			}()
		case message := <-bus.rpcFailedServers:
			// TODO: receives a msg that a server had failed
			// Notice: At this stage, the failed server has already been removed and detached
			go func() {
				logrus.Debugf("[SpineBus-%d] Using BfRt Client to remove server-related DP info from spine; serverID = %d", bus.ctrlID, message.Server.Id)
				if bus.topology != nil {
					bus.topology.Debug()
				}
			}()
		case message := <-bus.newLeaves:
			// TODO: receives a msg that a leaf was added
			// Notice: At this stage, the added leaf has already been added to the topology
			go func() {
				logrus.Debugf("[SpineBus-%d] Using BfRt Client to add leaf-related DP info at spine; leaf ID=%d, Index=%d",
					bus.ctrlID,
					message.Leaf.Id,
					message.Leaf.Index)
				if bus.topology != nil {
					bus.topology.Debug()
				}
			}()
		case message := <-bus.newServers:
			// TODO: receives a msg that a server was added
			// Notice: At this stage, the added server has already been added to the topology
			go func() {
				logrus.Debugf("[SpineBus-%d] Using BfRt Client to add server-related DP info at spine; serverID = %d", bus.ctrlID, message.Server.Id)
				if bus.topology != nil {
					bus.topology.Debug()
				}
			}()
		case message := <-bus.newVCs:
			// TODO: receives a msg that a VC was added
			go func() {
				if message.Type == horus_net.VCUpdateAdd {
					logrus.Debugf("[SpineBus-%d] Using BfRt Client to add VC-related DP info tp spine; VC ID = %d", bus.ctrlID, message.VCInfo.Id)
				} else if message.Type == horus_net.VCUpdateRem {
					logrus.Debugf("[SpineBus-%d] Using BfRt Client to remove VC-related DP info tp spine; VC ID = %d", bus.ctrlID, message.VCInfo.Id)
				}
				if bus.vcm != nil {
					bus.vcm.Debug()
				}
			}()

		default:
			continue
		}
	}
}

func (e *SpineBus) initialize() {
	logrus.Infof("[SpineBus-%d] Running initialization logic", e.ctrlID)
}

func (e *SpineBus) Start() {
	e.initialize()
	go e.processIngress()
	<-e.doneChan
}
