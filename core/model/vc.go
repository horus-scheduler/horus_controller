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
	leafDetached := false
	if server, ok := vc.Servers.Load(serverID); ok {
		leaf := server.Parent
		leafHasOtherChild := false
		for _, s := range leaf.Children {
			if s.ID != serverID {
				if _, ok1 := vc.Servers.Load(s.ID); ok1 {
					leafHasOtherChild = true
					break
				}
			}
		}
		if !leafHasOtherChild {
			vc.Leaves.Delete(leaf.ID)
			leafDetached = true
		}
		vc.Servers.Delete(serverID)
		return true, leafDetached
	}
	return false, leafDetached
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
		for _, server := range leaf.Children {
			logrus.Debug("Removing server: ", server.ID)
			serverDetached, leafDetach := vc.DetachServer(server.ID)
			if serverDetached {
				detachedServers = append(detachedServers, server)
			}
			logrus.Debug("Removed?: ", serverDetached)
			isDetached = isDetached || leafDetach
		}
	}
	logrus.Debug("LEAF REMOVED: ", isDetached)
	return isDetached, detachedServers
}

func (vc *VirtualCluster) Debug() {
	logrus.Debug("- VC ID: ", vc.ClusterID)
	logrus.Debug("-- Spines Count=", len(vc.Spines.internal))
	for _, spine := range vc.Spines.internal {
		logrus.Debug("-- Spine: ", spine.ID)
	}
	logrus.Debug("-- Leaves Count=", len(vc.Leaves.internal))
	for _, leaf := range vc.Leaves.internal {
		logrus.Debug("-- Leaf: ", leaf.ID, ", First WID: ", leaf.FirstWorkerID, ", Last WID: ", leaf.LastWorkerID)
	}
	logrus.Debug("--Servers Count=", len(vc.Servers.internal))
	for _, server := range vc.Servers.internal {
		logrus.Debug("-- Server: ", server.ID, ", First WID: ", server.FirstWorkerID, ", Last WID: ", server.LastWorkerID)
	}
}
