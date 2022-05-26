package model

import (
	"sync"

	"github.com/sirupsen/logrus"
)

type VirtualCluster struct {
	sync.RWMutex
	ClusterID uint16
	Active    bool
	Client    *Node
	Spines    *NodeMap // nodeID -> *Node
	Leaves    *NodeMap
	Servers   *NodeMap
}

func NewVC(vcConfig *vcConfig, topology *SimpleTopology) *VirtualCluster {
	s := &VirtualCluster{ClusterID: vcConfig.ID,
		Active: true,
		Client: nil,
	}
	s.Servers = NewNodeMap()
	s.Leaves = NewNodeMap()
	s.Spines = NewNodeMap()

	for _, spineID := range vcConfig.Spines {
		node := topology.GetNode(spineID, NodeType_Spine)
		if node != nil {
			s.Spines.Store(node.ID, node)
		}
	}

	for _, serverConfig := range vcConfig.Servers {
		node := topology.GetNode(serverConfig.ID, NodeType_Server)
		if node != nil {
			s.Servers.Store(node.ID, node)
			s.Leaves.Store(node.Parent.ID, node.Parent)
		}
	}

	return s
}

func (vc *VirtualCluster) Activate() {
	vc.Lock()
	defer vc.Unlock()
	vc.Active = true
}

func (vc *VirtualCluster) Deactivate() {
	vc.Lock()
	defer vc.Unlock()
	vc.Active = false
}

// Maybe?
// AddServer, AddLeaf, AddSpine
// RemoveLeaf, RemoveSpine

// Remove a server from the VC, and automatically remove a leaf if it has no other children in this VC
// This method does not change the underlying topology (First and Last workerID)
// To modify the topology, one should call SimpleTopology.RemoveServer(serverID)
func (vc *VirtualCluster) RemoveServer(serverID uint16) bool {
	// TODO: Should we remove Spines?
	if server, ok := vc.Servers.Load(serverID); ok {
		leaf := server.Parent
		leafHasOtherChild := false
		for _, s := range leaf.Children {
			if s.ID != serverID {
				if _, ok1 := vc.Servers.Load(s.ID); ok1 {
					leafHasOtherChild = true
				}
			}
		}
		if !leafHasOtherChild {
			vc.Leaves.Delete(leaf.ID)
		}
		vc.Servers.Delete(serverID)
		return true
	}
	return false
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
