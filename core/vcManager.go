package core

import (
	"strconv"
	"strings"
	"sync"

	"github.com/khaledmdiab/horus_controller/core/model"
	"github.com/sirupsen/logrus"
)

type vcMap struct {
	sync.RWMutex
	internal map[uint16]*model.VirtualCluster
}

func newVCMap() *vcMap {
	return &vcMap{
		internal: make(map[uint16]*model.VirtualCluster),
	}
}

func (rm *vcMap) Internal() map[uint16]*model.VirtualCluster {
	rm.Lock()
	defer rm.Unlock()
	return rm.internal
}

func (rm *vcMap) Load(key uint16) (value *model.VirtualCluster, ok bool) {
	rm.RLock()
	result, ok := rm.internal[key]
	rm.RUnlock()
	return result, ok
}

func (rm *vcMap) Delete(key uint16) {
	rm.Lock()
	delete(rm.internal, key)
	rm.Unlock()
}

func (rm *vcMap) Store(key uint16, value *model.VirtualCluster) {
	rm.Lock()
	rm.internal[key] = value
	rm.Unlock()
}

type vcListMap struct {
	sync.RWMutex
	internal map[uint16][]*model.VirtualCluster // nodeID -> []*VirtualCluster
}

func newVCListMap() *vcListMap {
	return &vcListMap{
		internal: make(map[uint16][]*model.VirtualCluster),
	}
}

func (rm *vcListMap) LoadVCList(key uint16) ([]*model.VirtualCluster, bool) {
	rm.RLock()
	result, ok := rm.internal[key]
	rm.RUnlock()
	return result, ok
}

func (rm *vcListMap) DeleteVCList(key uint16) {
	rm.Lock()
	delete(rm.internal, key)
	rm.Unlock()
}

func (rm *vcListMap) AppendToVCList(key uint16, vc *model.VirtualCluster) {
	rm.Lock()
	rm.internal[key] = append(rm.internal[key], vc)
	rm.Unlock()
}

func (rm *vcListMap) RemoveFromVCList(key uint16, vc *model.VirtualCluster) {
	rm.Lock()
	defer rm.Unlock()

	if vcList, ok := rm.internal[key]; ok {
		var vcIdx int = -1
		// Linear search.
		// TODO: Could do better if we keep an updated list of indices
		for currIdx, currVC := range vcList {
			if currVC.ClusterID == vc.ClusterID {
				vcIdx = currIdx
				break
			}
		}

		if vcIdx >= 0 {
			rm.internal[key] =
				append(rm.internal[key][:vcIdx],
					rm.internal[key][vcIdx+1:]...)
		}
	}
}

// Manages all VCs in the system
type VCManager struct {
	vcs       *vcMap     // clusterID -> *VirtualCluster
	serverVCs *vcListMap // serverID -> []*VirtualCluster: all VCs belonging to this server
	leafVCs   *vcListMap // leafID -> []*VirtualCluster: all VCs belonging to this leaf
	topology  *model.Topology
}

func NewVCManager(topology *model.Topology) *VCManager {
	return &VCManager{
		vcs:       newVCMap(),
		serverVCs: newVCListMap(),
		leafVCs:   newVCListMap(),
		topology:  topology, // TODO: Do we need topology?
	}
}

func (vcm *VCManager) GetVCs() []*model.VirtualCluster {
	// vcm.vcs.RLock()
	// defer vcm.vcs.RUnlock()
	var vcs []*model.VirtualCluster
	for _, vc := range vcm.vcs.Internal() {
		vcs = append(vcs, vc)
	}
	return vcs
}

// Does not (and shouldn't) modify the topology
func (vcm *VCManager) AddVC(vc *model.VirtualCluster) {
	// ClusterID -> *VirtualCluster
	vcm.vcs.Store(vc.ClusterID, vc)

	// ServerID -> []*VirtualCluster
	for _, s := range vc.Servers.Internal() {
		s.Lock()
		vcm.serverVCs.AppendToVCList(s.ID, vc)
		s.Unlock()
	}

	// LeafID -> []*VirtualCluster
	for _, l := range vc.Leaves.Internal() {
		vcm.leafVCs.AppendToVCList(l.ID, vc)
	}
}

// Does not (and shouldn't) modify the topology
func (vcm *VCManager) RemoveVC(vc *model.VirtualCluster) {
	logrus.Debug("Removing VC: ", vc.ClusterID, "...")
	// ClusterID -> *VirtualCluster
	vcm.vcs.Delete(vc.ClusterID)

	// ServerID -> []*VirtualCluster
	for _, s := range vc.Servers.Internal() {
		vcm.serverVCs.RemoveFromVCList(s.ID, vc)
	}

	// LeafID -> []*VirtualCluster
	for _, l := range vc.Leaves.Internal() {
		vcm.leafVCs.RemoveFromVCList(l.ID, vc)
	}
}

// Usage: get which VCs are impacted by a server failure
func (vcm *VCManager) GetVCsOfServer(serverID uint16) ([]*model.VirtualCluster, bool) {
	return vcm.serverVCs.LoadVCList(serverID)
}

// Usage: get which VCs are impacted by a leaf failure
func (vcm *VCManager) GetVCsOfLeaf(leafID uint16) ([]*model.VirtualCluster, bool) {
	return vcm.leafVCs.LoadVCList(leafID)
}

// Detaches a server from the VC Manager, and from every VC in the system
func (vcm *VCManager) DetachServer(serverID uint16) bool {
	vcs, found := vcm.GetVCsOfServer(serverID)
	logrus.Debugf("[VC Mgr] Detaching server %d, found? %t", serverID, found)
	logrus.Debugf("[VC Mgr] VCs count %d", len(vcs))
	detached := false
	firstDetach := false
	if found {
		for _, vc := range vcs {
			serverDetached, leafDetached := vc.DetachServer(serverID)
			if !firstDetach {
				detached = serverDetached
				firstDetach = true
			} else {
				detached = detached && serverDetached
			}
			server := vcm.topology.GetNode(serverID, model.NodeType_Server)
			if leafDetached && server.Parent != nil {
				logrus.Debugf("[VC Mgr] Removing VC %d from leaf %d after detaching server %d", vc.ClusterID, server.Parent.ID, serverID)
				vcm.leafVCs.RemoveFromVCList(server.Parent.ID, vc)
			}
		}
	}
	vcm.serverVCs.DeleteVCList(serverID)
	logrus.Debugf("[VC Mgr] server %d, detached? %t", serverID, detached)

	return found && detached
}

// Detaches a leaf from the VC Manager, and from every VC in the system
func (vcm *VCManager) DetachLeaf(leafID uint16) bool {
	vcs, found := vcm.GetVCsOfLeaf(leafID)
	logrus.Debug("[VC Mgr] leaf "+strconv.Itoa(int(leafID))+" found? ", found)
	detached := true
	if found {
		// For each VC: detach the leaf and servers
		for _, vc := range vcs {
			leafDetached, detachedServers := vc.DetachLeaf(leafID)
			detached = detached && leafDetached
			for _, srv := range detachedServers {
				vcm.serverVCs.RemoveFromVCList(srv.ID, vc)
			}
		}
	}
	vcm.leafVCs.DeleteVCList(leafID)
	return found && detached
}

func (vcm *VCManager) Debug() {
	logrus.Debug("VC Manager")
	logrus.Debug("VC Count: ", len(vcm.vcs.internal))
	for _, vc := range vcm.vcs.internal {
		vc.Debug()
	}
	logrus.Debug()
	for sID, vcs := range vcm.serverVCs.internal {
		if len(vcs) > 0 {
			logrus.Debug("-- Server ID: ", sID)
			var line []string
			for _, vc := range vcs {
				line = append(line, strconv.Itoa(int(vc.ClusterID)))
			}
			logrus.Debug("---- VCs: ", strings.Join(line, ", "))
			logrus.Debug()
		}

	}

	for lID, vcs := range vcm.leafVCs.internal {
		if len(vcs) > 0 {
			logrus.Debug("-- Leaf ID: ", lID)
			var line []string
			for _, vc := range vcs {
				line = append(line, strconv.Itoa(int(vc.ClusterID)))
			}
			logrus.Debug("---- VCs: ", strings.Join(line, ", "))
			logrus.Debug()
		}
	}
}

/*
func (sm *SessionManager) AddReceiver(sessionAddress string, receiver *model.Node) *model.MulticastTree {
	if s, found := sm.sessions.Load(sessionAddress); found {
		s.AddReceiver(receiver)
		return sm.algorithm.OnReceiverAdded(s, receiver)
	}
	return nil
}

func (sm *SessionManager) RemoveReceiver(sessionAddress string, receiver *model.Node) *model.MulticastTree {
	if s, found := sm.sessions.Load(sessionAddress); found {
		s.RemoveReceiver(receiver)
		return sm.algorithm.OnReceiverRemoved(s, receiver)
	}
	return nil
}

func (sm *SessionManager) RemoveReceiverByAddress(sessionAddress string, receiverAddress string) *model.MulticastTree {
	if s, found := sm.sessions.Load(sessionAddress); found {
		s.RemoveReceiverByAddress(receiverAddress)
		return sm.algorithm.OnReceiverRemovedByAddress(s, receiverAddress)
	}
	return nil
}
*/
