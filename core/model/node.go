package model

import (
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
	PortId      uint16 // valid if Type == server | leaf
	Type        NodeType
	Parent      *Node
	Children    []*Node

	FirstWorkerID uint16 // valid if Type == leaf | server
	LastWorkerID  uint16 // valid if Type == leaf | server
	Index         uint16 // valid if Type == leaf

	LastPingTime time.Time
	Healthy      bool
	Ready        bool
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
