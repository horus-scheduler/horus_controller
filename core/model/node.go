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
	Address  string
	ID       uint16
	PortId   uint16 // valid if Type == server
	Type     NodeType
	Parent   *Node
	Children []*Node

	FirstWorkerID uint16 // valid if Type == leaf | server
	LastWorkerID  uint16 // valid if Type == leaf | server
}

type TrackableNode struct {
	*Node
	LastPingTime time.Time
	Healthy      bool
	Ready        bool
}

func NewNode(address string, id, portId uint16, nodeType NodeType) *Node {
	return &Node{Address: address,
		ID:       id,
		PortId:   portId,
		Type:     nodeType,
		Parent:   nil,
		Children: make([]*Node, 0),
	}
}

func NewTrackableNode(node *Node) *TrackableNode {
	return &TrackableNode{Node: node,
		LastPingTime: time.Now(),
		Healthy:      true,
		Ready:        false,
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
	rm.RLock()
	defer rm.RUnlock()
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

type TrackableNodeMap struct {
	sync.RWMutex
	// TODO: shouldn't be public!
	Internal map[string]*TrackableNode
}

func NewTrackableNodeMap() *TrackableNodeMap {
	return &TrackableNodeMap{
		Internal: make(map[string]*TrackableNode),
	}
}

func (rm *TrackableNodeMap) Load(key string) (value *TrackableNode, ok bool) {
	rm.RLock()
	result, ok := rm.Internal[key]
	rm.RUnlock()
	return result, ok
}

func (rm *TrackableNodeMap) Delete(key string) {
	rm.Lock()
	delete(rm.Internal, key)
	rm.Unlock()
}

func (rm *TrackableNodeMap) Store(key string, value *TrackableNode) {
	rm.Lock()
	rm.Internal[key] = value
	rm.Unlock()
}
