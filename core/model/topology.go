package model

import (
	"errors"
	"strconv"
	"sync"

	horus_pb "github.com/khaledmdiab/horus_controller/protobuf"
	"github.com/sirupsen/logrus"
)

type Topology struct {
	sync.RWMutex
	Root    *Node
	Servers *NodeMap
	Leaves  *NodeMap
	Spines  *NodeMap
	Cores   *NodeMap
}

func NewDCNTopology(topoCfg *topoRootConfig) *Topology {
	s := &Topology{
		Servers: NewNodeMap(),
		Leaves:  NewNodeMap(),
		Spines:  NewNodeMap(),
		Cores:   NewNodeMap(),
	}

	s.Root = NewNode("", "", 0, 0, NodeType_Core)
	s.Root.Parent = nil
	s.Cores.Store(s.Root.ID, s.Root)

	for _, spineConf := range topoCfg.Spines {
		spine := NewNode(spineConf.Address, "", spineConf.ID, 0, NodeType_Spine)
		spine.Parent = s.Root
		s.Root.Children = append(s.Root.Children, spine)
		s.Spines.Store(spine.ID, spine)

		for _, leafID := range spineConf.LeafIDs {
			// workerIDs are local per leaf
			var workerID uint16 = 0
			leafConf := topoCfg.Leaves[leafID]
			leaf := NewNode(leafConf.Address, leafConf.MgmtAddress, leafConf.ID, 0, NodeType_Leaf)
			leaf.Parent = spine
			spine.Children = append(spine.Children, leaf)
			leaf.FirstWorkerID = workerID
			for _, serverID := range leafConf.ServerIDs {
				serverConf := topoCfg.Servers[serverID]
				server := NewNode(serverConf.Address, "", serverConf.ID, serverConf.PortID, NodeType_Server)
				server.Parent = leaf
				leaf.Children = append(leaf.Children, server)
				// Set worker IDs per Server
				// worker IDs are contigous for Servers belonging to the same Leaf
				server.FirstWorkerID = workerID
				workerID += serverConf.WorkersCount
				server.LastWorkerID = workerID - 1

				s.Servers.Store(server.ID, server)
			}
			leaf.LastWorkerID = workerID - 1
			s.Leaves.Store(leaf.ID, leaf)
		}
	}

	return s
}

func (s *Topology) GetNode(nodeId uint16, nodeType NodeType) *Node {
	var nodes *NodeMap
	if nodeType == NodeType_Server {
		nodes = s.Servers
	} else if nodeType == NodeType_Leaf {
		nodes = s.Leaves
	} else if nodeType == NodeType_Spine {
		nodes = s.Spines
	}

	node, found := nodes.Load(nodeId)
	if found {
		return node
	}
	return nil
}

// Assumes that addresses are unique per Servers, Leaves, and Spines
func (s *Topology) GetNodeByAddress(nodeAddress string, nodeType NodeType) *Node {
	var nodes *NodeMap
	if nodeType == NodeType_Server {
		nodes = s.Servers
	} else if nodeType == NodeType_Leaf {
		nodes = s.Leaves
	} else if nodeType == NodeType_Spine {
		nodes = s.Spines
	}
	for _, n := range nodes.Internal() {
		if n.Address == nodeAddress {
			return n
		}
	}
	return nil
}

func (s *Topology) AddServerToLeaf(serverInfo *horus_pb.ServerInfo, leafID uint16) (*Node, error) {
	leaf := s.GetNode(leafID, NodeType_Leaf)
	existingServer := s.GetNode(uint16(serverInfo.Id), NodeType_Server)
	if leaf == nil {
		return nil, errors.New("leaf " + strconv.Itoa(int(leafID)) + " doesn't exist!")
	}
	if existingServer != nil {
		if existingServer.Parent == leaf {
			logrus.Warnf("[Topology] Server %d already exists", existingServer.ID)
			return nil, errors.New("server " + strconv.Itoa(int(serverInfo.Id)) + " already exists")
		} else {
			return nil, errors.New("server " + strconv.Itoa(int(serverInfo.Id)) + " already exists with mismatched leaf")
		}
	}
	leaf.Lock()
	defer leaf.Unlock()

	server := NewNode(serverInfo.Address, "", uint16(serverInfo.Id), uint16(serverInfo.PortId), NodeType_Server)
	server.Parent = leaf
	leaf.Children = append(leaf.Children, server)
	workerID := leaf.LastWorkerID + 1
	server.FirstWorkerID = workerID
	workerID += uint16(serverInfo.WorkersCount)
	server.LastWorkerID = workerID - 1
	leaf.LastWorkerID = server.LastWorkerID
	s.Servers.Store(server.ID, server)
	return server, nil
}

// Input: Global serverID
// Output: a list of updated servers and boolean indicating whether it's removed
func (s *Topology) RemoveServer(serverID uint16) (bool, []*Node) {
	logrus.Debugf("[Topology] Removing server %d", serverID)
	server := s.GetNode(serverID, NodeType_Server)
	var updatedServers []*Node
	if server == nil {
		logrus.Debugf("[Topology] Server %d isn't found", serverID)
		return false, updatedServers
	}
	s.Lock()
	defer s.Unlock()

	leaf := server.Parent
	var workerID uint16 = 0
	var removedIdx int = -1
	for sIdx, srv := range leaf.Children {
		if srv.ID != serverID {
			workersCount := srv.LastWorkerID - srv.FirstWorkerID + 1
			srv.FirstWorkerID = workerID
			workerID += workersCount
			srv.LastWorkerID = workerID - 1
			if removedIdx > -1 {
				updatedServers = append(updatedServers, srv)
			}
		} else {
			removedIdx = sIdx
		}
	}

	leaf.FirstWorkerID = 0
	if workerID == 0 {
		leaf.LastWorkerID = 0
	} else {
		leaf.LastWorkerID = workerID - 1
	}
	logrus.Debug("[Topology] Updating leaf's children")
	leaf.Lock()
	leaf.Children = append(leaf.Children[:removedIdx], leaf.Children[removedIdx+1:]...)
	leaf.Unlock()
	logrus.Debug("[Topology] Leaf's children are updated")
	s.Servers.Delete(serverID)
	logrus.Debugf("[Topology] Number of servers to be updated = %d", len(updatedServers))
	return true, updatedServers
}

// Return the local index of the removed leaf within the spine
// Input: Global leafID
func (s *Topology) RemoveLeaf(leafID uint16) int {
	leaf := s.GetNode(leafID, NodeType_Leaf)

	if leaf == nil {
		return -1
	}
	logrus.Debug("Removing leaf: ", leaf.ID)
	// Remove children *before* the lock
	for {
		if len(leaf.Children) == 0 {
			break
		}
		s.RemoveServer(leaf.Children[0].ID)
	}
	s.Lock()
	defer s.Unlock()

	spine := leaf.Parent
	var removedIdx int = 0
	for lIdx, l := range spine.Children {
		if l.ID == leafID {
			removedIdx = lIdx
			break
		}
	}
	spine.Children = append(spine.Children[:removedIdx], spine.Children[removedIdx+1:]...)
	s.Leaves.Delete(leafID)

	return removedIdx
}

func (s *Topology) Debug() {
	logrus.Debug("Spines Count=", len(s.Spines.Internal()))
	logrus.Debug("Leaves Count=", len(s.Leaves.Internal()))
	logrus.Debug("Servers Count=", len(s.Servers.Internal()))

	// DFS
	stack := make([]*Node, 0)
	stack = append(stack, s.Root)
	var node *Node
	for {
		if len(stack) == 0 {
			break
		}
		node, stack = stack[len(stack)-1], stack[:len(stack)-1]
		if node.Type == NodeType_Spine {
			logrus.Debug("- Spine: ", node.ID)
		} else if node.Type == NodeType_Leaf {
			logrus.Debug("-- Leaf: ", node.ID, ", First WID: ", node.FirstWorkerID, ", Last WID: ", node.LastWorkerID)
		} else if node.Type == NodeType_Server {
			logrus.Debug("--- Server: ", node.ID, ", First WID: ", node.FirstWorkerID, ", Last WID: ", node.LastWorkerID)
		}
		for i := len(node.Children) - 1; i >= 0; i-- {
			stack = append(stack, node.Children[i])
		}
	}
}
