package model

import (
	"sync"
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
	CoreNodes []*Node
	AggNodes  []*Node
	TorNodes  []*Node
	HostNodes []*Node
	TorCount  int
}

func NewSimpleTopology(torAddresses []string) *SimpleTopology {
	torCount := len(torAddresses)
	s := &SimpleTopology{
		CoreNodes: make([]*Node, 0),
		AggNodes:  make([]*Node, 0),
		TorNodes:  make([]*Node, 0),
		HostNodes: make([]*Node, 0),
		TorCount:  len(torAddresses),
	}

	aggNode := NewNode("", 0, 0)
	s.AggNodes = append(s.AggNodes, aggNode)
	for torId := 0; torId < torCount; torId += 1 {
		torNode := NewNode(torAddresses[torId], uint32(torId), 0)
		torNode.Parents = append(torNode.Parents, aggNode)
		aggNode.Children = append(aggNode.Children, torNode)
		s.TorNodes = append(s.TorNodes, torNode)
		for hostId := 0; hostId < torCount; hostId += 1 {
			hostNode := NewNode("", uint32(torCount*torId+hostId), 0)
			hostNode.Parents = append(hostNode.Parents, torNode)
			torNode.Children = append(torNode.Children, hostNode)
			s.HostNodes = append(s.HostNodes, hostNode)
		}
	}

	return s
}

func (s *SimpleTopology) GetNode(nodeId uint32, nodeType TopologyNodeType) *Node {
	var nodes []*Node
	if nodeType == AggNode {
		nodes = s.AggNodes
	} else if nodeType == TorNode {
		nodes = s.TorNodes
	} else if nodeType == HostNode {
		nodes = s.HostNodes
	}
	if nodeId < 0 || nodeId >= uint32(len(nodes)) {
		return nil
	}
	return nodes[nodeId]
}

func (s *SimpleTopology) GetNodeByAddress(nodeAddress string, nodeType TopologyNodeType) *Node {
	var nodes []*Node
	if nodeType == AggNode {
		nodes = s.AggNodes
	} else if nodeType == TorNode {
		nodes = s.TorNodes
	} else if nodeType == HostNode {
		nodes = s.HostNodes
	}
	for _, n := range nodes {
		if n.Address == nodeAddress {
			return n
		}
	}
	return nil
}
