package ctrl_mgr

import (
	"context"
	"fmt"
	"math"
	"sync"
	"time"

	"github.com/horus-scheduler/horus_controller/core"
	"github.com/horus-scheduler/horus_controller/core/model"
	horus_net "github.com/horus-scheduler/horus_controller/core/net"
	bfrtC "github.com/khaledmdiab/bfrt-go-client/pkg/client"
	"github.com/sirupsen/logrus"
)

type switchManager struct {
	sync.RWMutex
	topoFp string
	cfg    *rootConfig
	status *core.CtrlStatus

	// A single end point per phy. switch to handles pkts to/from the ASIC
	asicEndPoint *asicEndPoint

	rpcEndPoint  *horus_net.ManagerRpcEndpoint
	failedLeaves chan *horus_net.LeafFailedMessage
	newLeaves    chan *horus_net.LeafAddedMessage

	leaves []*leafController
	spines []*spineController
}

type SwitchManagerOption func(*switchManager)

func initLogger(cfg *rootConfig) {
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

func NewSwitchManager(topoFp, cfgFp string, opts ...SwitchManagerOption) *switchManager {
	logrus.SetLevel(logrus.TraceLevel)
	// Read configuration
	cfg := ReadConfigFile(cfgFp)

	// Initialize program-related structures
	initLogger(cfg)
	horus_net.InitHorusDefinitions()

	// Initialize program status
	status := core.NewCtrlStatus()

	// Initialize leaf and spine controllers
	var leaves []*leafController
	var spines []*spineController
	for _, leafCtrl := range cfg.Leaves {
		newCtrl := NewBareLeafController(leafCtrl.ID, leafCtrl.PipeID, cfg)
		leaves = append(leaves, newCtrl)
	}
	for _, spineCtrl := range cfg.Spines {
		newCtrl := NewSpineController(spineCtrl.ID, spineCtrl.PipeID, topoFp, cfg)
		spines = append(spines, newCtrl)
	}

	// ASIC <-> CPU interface.
	asicIngress := make(chan []byte, horus_net.DefaultUnixSockSendSize)
	asicEgress := make(chan []byte, horus_net.DefaultUnixSockSendSize)
	failedLeaves := make(chan *horus_net.LeafFailedMessage, horus_net.DefaultUnixSockSendSize)
	newLeaves := make(chan *horus_net.LeafAddedMessage, horus_net.DefaultUnixSockSendSize)
	asicEndPoint := NewAsicEndPoint(cfg.AsicIntf, leaves, spines, asicIngress, asicEgress)
	rpcEndPoint := horus_net.NewManagerRpcEndpoint(cfg.MgmtAddress, failedLeaves, newLeaves)
	s := &switchManager{
		topoFp:       topoFp,
		cfg:          cfg,
		status:       status,
		asicEndPoint: asicEndPoint,
		rpcEndPoint:  rpcEndPoint,
		failedLeaves: failedLeaves,
		newLeaves:    newLeaves,
		leaves:       leaves,
		spines:       spines,
	}

	for _, opt := range opts {
		opt(s)
	}

	s.init(cfg)

	return s
}

func (sc *switchManager) init_random_adjust_tables(bfrtclient *bfrtC.Client) {
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

func (sc *switchManager) init_common_leaf_bfrt_setup(bfrtclient *bfrtC.Client) {
	logrus.Debugf("[Manager] Setting up common leaf table entries")

	ctx := context.Background()

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
	sc.init_random_adjust_tables(bfrtclient)
}

func (sc *switchManager) init(cfg *rootConfig) {
	logrus.Debugf("[Manager] Setting up switches table entries")
	// Parham: Below are general leaf entries (common for all virtual leaves), so I used first leaf instance as client
	// bfrtclient := sc.leaves[0].bfrt
	// sc.init_common_leaf_bfrt_setup(bfrtclient)
}

func (sc *switchManager) Run() {
	// ASIC connections
	go sc.asicEndPoint.Start()
	go sc.rpcEndPoint.Start()
	time.Sleep(time.Second)

	// Start Spine and Leaf controllers
	for _, s := range sc.spines {
		go s.Start()
	}
	time.Sleep(time.Second)

	for _, l := range sc.leaves {
		go l.StartBare(true)
	}

	for {
		select {
		case newLeaf := <-sc.newLeaves:
			leafID := uint16(newLeaf.Leaf.Id)
			logrus.Debugf("[Manager] Adding leaf %d", leafID)
			sc.Lock()
			logrus.Debugf("[Manager] Leaves count = %d", len(sc.leaves))
			// leafCtrl := NewLeafController(leafID, sc.topoFp, sc.cfg,
			// 	WithoutLeaves(),
			// 	WithExtraLeaf(newLeaf.Leaf))
			leafCtrl := NewBareLeafController(leafID, newLeaf.Leaf.PipeID, sc.cfg)
			err := leafCtrl.FetchTopology()
			if err == nil {
				leaf := leafCtrl.topology.GetNode(leafID, model.NodeType_Leaf)
				if leaf != nil {
					sc.leaves = append(sc.leaves, leafCtrl)
					sc.asicEndPoint.AddLeafCtrl(leafCtrl)
					go leafCtrl.StartBare(false)
				} else {
					logrus.Errorf("[Manager] Leaf %d was not added", leafID)
				}
			}
			logrus.Debugf("[Manager] Leaves count = %d", len(sc.leaves))
			sc.Unlock()

		case failedLeaf := <-sc.failedLeaves:
			logrus.Debugf("[Manager] Removing leaf %d", failedLeaf.Leaf.Id)
			var removedLeaf *leafController = nil
			removedLeafIdx := -1
			for leafIdx, leaf := range sc.leaves {
				if leaf.ID == uint16(failedLeaf.Leaf.Id) {
					removedLeaf = leaf
					removedLeafIdx = leafIdx
				}
			}
			if removedLeafIdx > -1 {
				removedLeaf.Shutdown()
				time.Sleep(time.Second)
				sc.Lock()
				logrus.Debugf("[Manager] Leaves count = %d", len(sc.leaves))
				sc.asicEndPoint.RemoveLeafCtrl(removedLeaf)
				sc.leaves = append(sc.leaves[:removedLeafIdx], sc.leaves[removedLeafIdx+1:]...)
				logrus.Debugf("[Manager] Leaves count = %d", len(sc.leaves))
				sc.Unlock()
			}
		default:
			continue
		}
	}
}
