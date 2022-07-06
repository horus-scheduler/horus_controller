package model

import (
	"errors"
	"strconv"
	"sync"

	horus_pb "github.com/horus-scheduler/horus_controller/protobuf"
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

func NewVC(vcConfig *horus_pb.VCInfo, topology *Topology) (*VirtualCluster, error) {
	vc := &VirtualCluster{ClusterID: uint16(vcConfig.Id)}
	vc.Servers = NewNodeMap()
	vc.Leaves = NewNodeMap()
	vc.Spines = NewNodeMap()

	for _, spineID := range vcConfig.Spines {
		spine := topology.GetNode(uint16(spineID), NodeType_Spine)
		if spine != nil {
			vc.Spines.Store(spine.ID, spine)
		} else {
			return nil, errors.New("spine " + strconv.Itoa(int(spineID)) + " doesn't exist")
		}
	}

	for _, serverConfig := range vcConfig.Servers {
		server := topology.GetNode(uint16(serverConfig.Id), NodeType_Server)
		if server != nil {
			leaf := server.Parent
			if leaf != nil {
				vc.Servers.Store(server.ID, server)
				vc.Leaves.Store(server.Parent.ID, server.Parent)
			} else {
				return nil, errors.New("leaf doesn't exist")
			}
		} else {
			return nil, errors.New("server " + strconv.Itoa(int(serverConfig.Id)) + " doesn't exist")
		}
	}

	return vc, nil
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
		// detachLeaf := false
		for _, server := range leaf.Children {
			logrus.Debugf("[VC] Removing server %d", server.ID)
			serverDetached, detachLeaf := vc.DetachServer(server.ID)
			if serverDetached {
				detachedServers = append(detachedServers, server)
			}
			logrus.Debugf("[VC] Server %d Removed? %t", server.ID, serverDetached)
			isDetached = isDetached || detachLeaf
		}
		// if detachLeaf {
		vc.Leaves.Delete(leaf.ID)
		// }
	}
	logrus.Debugf("[VC] Leaf %d removed? %t", leafID, isDetached)
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
