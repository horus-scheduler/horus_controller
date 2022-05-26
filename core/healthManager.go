package core

import (
	"log"
	"strconv"
	"sync"
	"time"

	"github.com/khaledmdiab/horus_controller/core/model"
)

type NodeHealthManager struct {
	nodes              *model.TrackableNodeMap
	activeNode         *model.TrackableNode
	activeNodeLock     sync.RWMutex // used when changing the activeNode member
	activeNodeChan     chan *HealthManagerMsg
	healthyNodeTimeOut time.Duration
	doneChan           chan bool
}

func NewNodeHealthManager(activeNodeChan chan *HealthManagerMsg, healthyNodeTimeOut int64) *NodeHealthManager {
	return &NodeHealthManager{
		activeNodeChan:     activeNodeChan,
		healthyNodeTimeOut: time.Duration(healthyNodeTimeOut) * time.Millisecond,
		nodes:              model.NewTrackableNodeMap(),
		doneChan:           make(chan bool),
	}
}

func (hm *NodeHealthManager) getNodeAddress(address string, nodeId uint16) string {
	addr := address
	if addr == "" {
		addr = strconv.Itoa(int(nodeId))
	}
	return addr
}

func (hm *NodeHealthManager) canBeActiveNode(node *model.TrackableNode) bool {
	node.RLock()
	checkNode := node.Ready && node.Healthy
	node.RUnlock()

	if checkNode {
		hm.activeNodeLock.RLock()
		defer hm.activeNodeLock.RUnlock()

		if hm.activeNode == nil {
			return true
		} else {
			hm.activeNode.RLock()
			elapsedTime := time.Now().Sub(hm.activeNode.LastPingTime)
			//log.Println(">>> Elapsed Time = ", elapsedTime.String())
			inactive := !hm.activeNode.Healthy || !hm.activeNode.Ready || elapsedTime >= hm.healthyNodeTimeOut
			hm.activeNode.RUnlock()

			return inactive
		}
	}
	return false
}

func (hm *NodeHealthManager) setActiveNode(node *model.TrackableNode) {
	hm.activeNodeLock.Lock()
	defer hm.activeNodeLock.Unlock()

	if hm.activeNode != nil {
		hm.activeNode.Lock()
		hm.activeNode.Healthy = false
		hm.activeNode.Ready = false
		hm.activeNode.Unlock()
	}
	hm.activeNode = node
}

func (hm *NodeHealthManager) GetActiveNodesAddresses() []string {
	addresses := make([]string, 0)
	for nodeAddress, node := range hm.nodes.Internal {
		node.Lock()
		if node.Healthy && node.Ready {
			addresses = append(addresses, nodeAddress)
		}
		node.Unlock()
	}
	return addresses
}

func (hm *NodeHealthManager) OnNodePingRecv(address string, nodeId, portId uint16) {
	nodeAddr := hm.getNodeAddress(address, nodeId)
	// Get Node from the Node map
	node, found := hm.nodes.Load(nodeAddr)
	if !found {
		n := model.NewNode(nodeAddr, nodeId, portId, model.NodeType_Leaf)
		node = model.NewTrackableNode(n)
		hm.nodes.Store(nodeAddr, node)
	}

	// Update its internal properties
	node.Lock()
	node.LastPingTime = time.Now()
	node.Healthy = true
	// TODO This needs to be changed after implementing the logic of syncing agents when they wake up
	node.Ready = true
	node.Unlock()

	// Set Active Agent
	if hm.canBeActiveNode(node) {
		log.Println(">>> Set Active Node on Ping ", nodeId)
		hm.setActiveNode(node)
		hm.activeNodeChan <- NewActiveNodeMsg(false, nil)
	}

	// If first ping or the Node isn't ready: sync-state
}

func (hm *NodeHealthManager) ProcessNodes() {
	// check which nodes aren't ready

	for {
		//x1 := time.Now()
		hm.nodes.RLock()
		for _, node := range hm.nodes.Internal {
			//log.Println("Node Address: ", nodeAddress)
			//log.Println("Node ID: ", Node.Id)
			//log.Println("Healthy? ", Node.Healthy)
			//log.Println("Ready? ", Node.Ready)
			//log.Println("================================================================")

			// First - update node.Healthy and node.Ready
			node.Lock()
			readyNode := true
			elapsedTime := time.Now().Sub(node.LastPingTime)
			if elapsedTime >= hm.healthyNodeTimeOut {
				// TODO check if this is an active agent
				node.Healthy = false
				node.Ready = false
				readyNode = false
			}
			node.Unlock()

			// Second - setActiveNode
			if readyNode {
				if hm.canBeActiveNode(node) {
					log.Println(">>> Reset Active Node")
					hm.setActiveNode(node)
					hm.activeNodeChan <- NewActiveNodeMsg(false, nil)
				}
			}
		}
		hm.nodes.RUnlock()
		//log.Println(time.Now().Sub(x1).String())
		time.Sleep(2 * hm.healthyNodeTimeOut)
	}
}

func (hm *NodeHealthManager) Start() {
	go hm.ProcessNodes()
	<-hm.doneChan
}
