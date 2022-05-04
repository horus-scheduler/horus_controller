package model

import (
	"sync"
)

type MulticastTree struct {
	sync.RWMutex
	UpstreamTorNode *Node
	UpstreamAggNode *Node
	CoreNode        *Node

	SourceNode         *Node
	DownstreamAggNodes map[uint32]*Node
	DownstreamTorNodes map[uint32]*Node
	ReceiverNodes      map[uint32]*Node
}

func NewMulticastTree() *MulticastTree {
	return &MulticastTree{
		DownstreamAggNodes: make(map[uint32]*Node),
		DownstreamTorNodes: make(map[uint32]*Node),
		ReceiverNodes:      make(map[uint32]*Node),
	}
}
