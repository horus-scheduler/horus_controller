package model

import (
	"fmt"
	"sync"
	"time"
)

type NodeType int

const (
	NodeType_Server NodeType = iota
	NodeType_Leaf
	NodeType_Spine
	NodeType_Core
)

type Node struct {
	sync.RWMutex
	Address     string
	MgmtAddress string
	ID          uint16
	PortId      uint16 // valid if Type == server
	Type        NodeType
	Parent      *Node
	Children    []*Node

	FirstWorkerID uint16 // valid if Type == leaf | server
	LastWorkerID  uint16 // valid if Type == leaf | server

	LastPingTime time.Time
	Healthy      bool
	Ready        bool
}

func (n *Node) GetIndex() (uint64, error) {
	if n.Parent == nil {
		return 0, fmt.Errorf("node %d has no parent", n.ID)
	}
	var nodeIdx uint64
	for cIdx, node := range n.Parent.Children {
		if n.ID == node.ID {
			nodeIdx = uint64(cIdx)
			return uint64(nodeIdx), nil
		}
	}

	// This would happen if there is a bug in how the parent node tracks its children
	return 0, fmt.Errorf("node %d has no parent (possible bug)", n.ID)
}

func NewNode(address, mgmtAddress string, id, portId uint16, nodeType NodeType) *Node {
	return &Node{Address: address,
		MgmtAddress: mgmtAddress,
		ID:          id,
		PortId:      portId,
		Type:        nodeType,
		Parent:      nil,
		Children:    make([]*Node, 0),
	}
}

type NodeMap struct {
	sync.RWMutex
	internal map[uint16]*Node
}

func NewNodeMap() *NodeMap {
	return &NodeMap{
		internal: make(map[uint16]*Node),
	}
}

func (rm *NodeMap) Internal() map[uint16]*Node {
	rm.Lock()
	defer rm.Unlock()
	return rm.internal
}

func (rm *NodeMap) Load(key uint16) (value *Node, ok bool) {
	rm.RLock()
	result, ok := rm.internal[key]
	rm.RUnlock()
	return result, ok
}

func (rm *NodeMap) Delete(key uint16) {
	rm.Lock()
	delete(rm.internal, key)
	rm.Unlock()
}

func (rm *NodeMap) Store(key uint16, value *Node) {
	rm.Lock()
	rm.internal[key] = value
	rm.Unlock()
}
