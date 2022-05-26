package model

import (
	"sync"

	"github.com/sirupsen/logrus"
)

type TopologyNodeType int

const (
	CoreNode TopologyNodeType = iota
	AggNode
	TorNode
	HostNode
)

// TODO consider changing the internal slices to maps
type SimpleTopology struct {
	sync.RWMutex
	Root    *Node
	Servers []*Node
	Leaves  []*Node
	Spines  []*Node
	Cores   []*Node
}

func NewSimpleTopology(topoCfg *topoRootConfig) *SimpleTopology {
	s := &SimpleTopology{
		Servers: make([]*Node, 0),
		Leaves:  make([]*Node, 0),
		Spines:  make([]*Node, 0),
		Cores:   make([]*Node, 0),
	}

	s.Root = NewNode("", 0, 0, NodeType_Core)
	s.Root.Parent = nil
	s.Cores = append(s.Cores, s.Root)

	for _, spineConf := range topoCfg.Spines {
		spine := NewNode(spineConf.Address, spineConf.ID, 0, NodeType_Spine)
		spine.Parent = s.Root
		s.Root.Children = append(s.Root.Children, spine)
		s.Spines = append(s.Spines, spine)

		for _, leafID := range spineConf.LeafIDs {
			// workerIDs are local per leaf
			var workerID uint16 = 0
			leafConf := topoCfg.Leaves[leafID]
			leaf := NewNode(leafConf.Address, leafConf.ID, 0, NodeType_Leaf)
			leaf.Parent = spine
			spine.Children = append(spine.Children, leaf)
			leaf.FirstWorkerID = workerID
			for _, serverID := range leafConf.ServerIDs {
				serverConf := topoCfg.Servers[serverID]
				server := NewNode(serverConf.Address, serverConf.ID, serverConf.PortID, NodeType_Server)
				server.Parent = leaf
				leaf.Children = append(leaf.Children, server)
				// Set worker IDs per Server
				// worker IDs are contigous for Servers belonging to the same Leaf
				server.FirstWorkerID = workerID
				workerID += serverConf.WorkersCount
				server.LastWorkerID = workerID - 1

				s.Servers = append(s.Servers, server)
			}
			leaf.LastWorkerID = workerID - 1
			s.Leaves = append(s.Leaves, leaf)
		}
	}

	return s
}

func (s *SimpleTopology) GetNode(nodeId uint16, nodeType NodeType) *Node {
	var nodes []*Node
	if nodeType == NodeType_Server {
		nodes = s.Servers
	} else if nodeType == NodeType_Leaf {
		nodes = s.Leaves
	} else if nodeType == NodeType_Spine {
		nodes = s.Spines
	}
	if nodeId >= uint16(len(nodes)) {
		return nil
	}

	return nodes[nodeId]
}

// Assumes that addresses are unique per Servers, Leaves, and Spines
func (s *SimpleTopology) GetNodeByAddress(nodeAddress string, nodeType NodeType) *Node {
	var nodes []*Node
	if nodeType == NodeType_Server {
		nodes = s.Servers
	} else if nodeType == NodeType_Leaf {
		nodes = s.Leaves
	} else if nodeType == NodeType_Spine {
		nodes = s.Spines
	}
	for _, n := range nodes {
		if n.Address == nodeAddress {
			return n
		}
	}
	return nil
}

// Return the local index of the removed server within the leaf
// Global serverID
func (s *SimpleTopology) RemoveServer(serverID uint16) int {
	server := s.GetNode(serverID, NodeType_Server)
	if server == nil {
		return -1
	}
	s.Lock()
	defer s.Unlock()

	leaf := server.Parent
	var workerID uint16 = 0
	var removedIdx int = 0
	for sIdx, s := range leaf.Children {
		if s.ID != serverID {
			workersCount := s.LastWorkerID - s.FirstWorkerID + 1
			s.FirstWorkerID = workerID
			workerID += workersCount
			s.LastWorkerID = workerID - 1
		} else {
			removedIdx = sIdx
		}
	}
	leaf.FirstWorkerID = 0
	leaf.LastWorkerID = workerID - 1
	leaf.Children = append(leaf.Children[:removedIdx], leaf.Children[removedIdx+1:]...)
	s.Servers = append(s.Servers[:serverID], s.Servers[serverID+1:]...)

	// Modify global server IDs
	for i := int(serverID); i < len(s.Servers); i++ {
		s.Servers[i].ID -= 1
	}

	return removedIdx
}

func (s *SimpleTopology) Debug() {
	logrus.Debug("Spines Count=", len(s.Spines))
	logrus.Debug("Leaves Count=", len(s.Leaves))
	logrus.Debug("Servers Count=", len(s.Servers))

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
