package ctrl_mgr

import (
	"time"
	"github.com/horus-scheduler/horus_controller/core"
	"github.com/horus-scheduler/horus_controller/core/model"
	horus_net "github.com/horus-scheduler/horus_controller/core/net"
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
	cp        SpineCP
	doneChan  chan bool
	// Map to store pending leaves to be added (in case leaf has no children)
	pending_leaves map[uint16]int
}

// NewSpineBus ...
func NewSpineBus(ctrlID uint16,
	busChan *SpineBusChan,
	topology *model.Topology,
	vcm *core.VCManager,
	healthMgr *core.LeafHealthManager,
	cp SpineCP) *SpineBus {
	return &SpineBus{
		SpineBusChan:   busChan,
		ctrlID:         ctrlID,
		topology:       topology,
		vcm:            vcm,
		healthMgr:      healthMgr,
		cp:             cp,
		doneChan:       make(chan bool, 1),
		pending_leaves: make(map[uint16]int),
	}
}

func (bus *SpineBus) processIngress() {
	t_last_read := time.Now()
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
					bus.cp.OnLeafChange(uint64(message.Leaf.Id), uint64(message.Leaf.Index), false)
				}
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
					leaf := bus.topology.GetNode(uint16(message.Leaf.Id), model.NodeType_Leaf)
					if len(leaf.Children) > 0 { // Add the new leaf
						bus.cp.OnLeafChange(uint64(message.Leaf.Id), uint64(message.Leaf.Index), true)
					} else { // Will be added later
						logrus.Debugf("[SpineBus-%d] Leaf has no children, will be added once workers added leafID", bus.ctrlID)
						bus.pending_leaves[uint16(message.Leaf.Id)] = 1
					}
					bus.topology.Debug()
				}

			}()
		case message := <-bus.newServers:
			// TODO: receives a msg that a server was added
			// Notice: At this stage, the added server has already been added to the topology
			go func() {
				logrus.Debugf("[SpineBus-%d] Using BfRt Client to add server-related DP info at spine; serverID = %d", bus.ctrlID, message.Server.Id)
				if bus.topology != nil {
					server := bus.topology.GetNode(uint16(message.Server.Id), model.NodeType_Server)
					if bus.pending_leaves[server.Parent.ID] != 0 {
						// Parent leaf add msg have been received but havent' actually added yet, takes effect now
						logrus.Debugf("[SpineBus-%d] New server added to previously empty leaf, adding leaf now; ServerID= %d, LeafID= %d",
							bus.ctrlID,
							message.Server.Id,
							server.Parent.ID)
						bus.cp.OnLeafChange(uint64(server.Parent.ID), uint64(server.Parent.Index), true)
						delete(bus.pending_leaves, server.Parent.ID)
					}
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
			elapsed := time.Since(t_last_read).Seconds()
			if elapsed >= 10 {
				bus.cp.MonitorStats()
				t_last_read = time.Now()	
			}
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
