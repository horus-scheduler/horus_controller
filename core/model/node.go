package model

import (
	"sync"
	"time"
)

type Node struct {
	sync.RWMutex
	Address  string
	Id       uint32
	PortId   uint32
	Parents  []*Node
	Children []*Node
}

type TrackableNode struct {
	*Node
	LastPingTime time.Time
	Healthy      bool
	Ready        bool
}

func NewNode(address string, id, portId uint32) *Node {
	return &Node{Address: address,
		Id:       id,
		PortId:   portId,
		Parents:  make([]*Node, 0),
		Children: make([]*Node, 0),
	}
}

func NewTrackableNode(address string, id, portId uint32) *TrackableNode {
	return &TrackableNode{Node: NewNode(address, id, portId),
		LastPingTime: time.Now(),
		Healthy:      true,
		Ready:        false,
	}
}

type NodeMap struct {
	sync.RWMutex
	internal map[string]*Node
}

func NewNodeMap() *NodeMap {
	return &NodeMap{
		internal: make(map[string]*Node),
	}
}

func (rm *NodeMap) Load(key string) (value *Node, ok bool) {
	rm.RLock()
	result, ok := rm.internal[key]
	rm.RUnlock()
	return result, ok
}

func (rm *NodeMap) Delete(key string) {
	rm.Lock()
	delete(rm.internal, key)
	rm.Unlock()
}

func (rm *NodeMap) Store(key string, value *Node) {
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