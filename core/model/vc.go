package model

import (
	"sync"

	horus_pb "github.com/khaledmdiab/horus_controller/protobuf"
	"github.com/sirupsen/logrus"
)

type VirtualCluster struct {
	sync.RWMutex
	ClusterID uint16
	// Active    bool
	// Client    *Node
	Spines  *NodeMap // nodeID -> *Node
	Leaves  *NodeMap
	Servers *NodeMap
}

func NewVC(vcConfig *horus_pb.VCInfo, topology *Topology) *VirtualCluster {
	s := &VirtualCluster{ClusterID: uint16(vcConfig.Id)}
	s.Servers = NewNodeMap()
	s.Leaves = NewNodeMap()
	s.Spines = NewNodeMap()

	for _, spineID := range vcConfig.Spines {
		node := topology.GetNode(uint16(spineID), NodeType_Spine)
		if node != nil {
			s.Spines.Store(node.ID, node)
		}
	}

	for _, serverConfig := range vcConfig.Servers {
		node := topology.GetNode(uint16(serverConfig.Id), NodeType_Server)
		if node != nil {
			s.Servers.Store(node.ID, node)
			s.Leaves.Store(node.Parent.ID, node.Parent)
		}
	}

	return s
}

// Maybe?
// AddServer, AddLeaf, AddSpine
// RemoveSpine

// Detach a server from the VC, and automatically detach a leaf
// if it has no other children in this VC.
// This method does not change the underlying topology
// (e.g., First and Last workerID)
// To modify the topology, one should call:
// SimpleTopology.RemoveServer(serverID)
// Returns:
// bool: the server is detached
// bool: a leaf is detached
func (vc *VirtualCluster) DetachServer(serverID uint16) (bool, bool) {
	detachLeaf := false
	server, ok := vc.Servers.Load(serverID)
	if !ok {
		return false, detachLeaf
	}

	leaf := server.Parent
	leafHasOtherChild := false
	for _, s := range leaf.Children {
		if s.ID != serverID {
			_, ok1 := vc.Servers.Load(s.ID)
			if ok1 {
				leafHasOtherChild = true
				break
			}
		}
	}
	// vc.Leaves.Delete(leaf.ID)
	detachLeaf = !leafHasOtherChild

	vc.Servers.Delete(serverID)
	return true, detachLeaf
}

// Detach a leaf from the VC, and automatically detach all servers.
// This method does not change the underlying topology
// (e.g., First and Last workerID)
// To modify the topology, one should call:
// SimpleTopology.RemoveLeaf(leafID)
func (vc *VirtualCluster) DetachLeaf(leafID uint16) (bool, []*Node) {
	isDetached := false
	var detachedServers []*Node
	if leaf, ok := vc.Leaves.Load(leafID); ok {
		detachLeaf := false
		for _, server := range leaf.Children {
			logrus.Debugf("[VC] Removing server %d", server.ID)
			serverDetached, detachLeaf := vc.DetachServer(server.ID)
			if serverDetached {
				detachedServers = append(detachedServers, server)
			}
			logrus.Debugf("[VC] Server %d Removed? %t", server.ID, serverDetached)
			isDetached = isDetached || detachLeaf
		}
		if detachLeaf {
			vc.Leaves.Delete(leaf.ID)
		}
	}
	logrus.Debug("[VC] Leaf %d removed? %t", leafID, isDetached)
	return isDetached, detachedServers
}

func (vc *VirtualCluster) Debug() {
	logrus.Debug("- VC ID: ", vc.ClusterID)
	logrus.Debug("-- Spines Count=", len(vc.Spines.Internal()))
	for _, spine := range vc.Spines.Internal() {
		logrus.Debug("-- Spine: ", spine.ID)
	}
	logrus.Debug("-- Leaves Count=", len(vc.Leaves.Internal()))
	for _, leaf := range vc.Leaves.Internal() {
		logrus.Debug("-- Leaf: ", leaf.ID, ", First WID: ", leaf.FirstWorkerID, ", Last WID: ", leaf.LastWorkerID)
	}
	logrus.Debug("--Servers Count=", len(vc.Servers.Internal()))
	for _, server := range vc.Servers.Internal() {
		logrus.Debug("-- Server: ", server.ID, ", First WID: ", server.FirstWorkerID, ", Last WID: ", server.LastWorkerID)
	}
}
