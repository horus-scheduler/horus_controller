package core

import (
	"errors"
	"strconv"
	"time"

	"github.com/khaledmdiab/horus_controller/core/model"
)

// Tracks the health of its servers
type LeafHealthManager struct {
	hmEgress           chan *LeafHealthMsg
	leaf               *model.Node
	topology           *model.Topology
	vcMgr              *VCManager
	healthyNodeTimeOut time.Duration
	doneChan           chan bool
}

func NewLeafHealthManager(leafID uint16,
	hmEgress chan *LeafHealthMsg,
	topology *model.Topology,
	vcMgr *VCManager,
	healthyNodeTimeOut int64) (*LeafHealthManager, error) {
	leaf, ok := topology.Leaves.Load(leafID)
	if !ok {
		return nil, errors.New("Leaf " + strconv.Itoa(int(leafID)) + " doesn't exist!")
	}
	return &LeafHealthManager{
		leaf:               leaf,
		hmEgress:           hmEgress,
		topology:           topology,
		vcMgr:              vcMgr,
		healthyNodeTimeOut: time.Duration(healthyNodeTimeOut) * time.Millisecond,
		doneChan:           make(chan bool),
	}, nil
}

func (hm *LeafHealthManager) processFailedNodes(nodes []*model.Node) {
	// Each node is a server.
	updatedMap := model.NewNodeMap()
	updated := make([]*model.Node, 0)
	for _, server := range nodes {
		updatedMap.Delete(server.ID)
		// Remove the server from *all* VCs
		hm.vcMgr.DetachServer(server.ID)
		// Remove the server from the topology
		us := hm.topology.RemoveServer(server.ID)
		for _, s := range us {
			updatedMap.Store(s.ID, s)
		}
	}
	for _, server := range updatedMap.Internal() {
		updated = append(updated, server)
	}
	if len(updated) > 0 {
		hm.hmEgress <- NewLeafHealthMsg(true, updated)
	}
}

// Logic for receiving a ping pkt from a server
func (hm *LeafHealthManager) OnNodePingRecv(serverId, portId uint16) {
	server, ok := hm.topology.Servers.Load(serverId)
	if !ok {
		return
		// TODO: should we create it?
	}

	// Update its internal properties
	server.Lock()
	server.LastPingTime = time.Now()
	server.Healthy = true
	server.Ready = true
	server.Unlock()
}

func (hm *LeafHealthManager) processNodes() {
	for {
		var failedServers []*model.Node
		hm.leaf.RLock()
		for _, server := range hm.leaf.Children {
			server.Lock()
			elapsedTime := time.Since(server.LastPingTime)
			if elapsedTime >= hm.healthyNodeTimeOut {
				server.Healthy = false
				server.Ready = false
				failedServers = append(failedServers, server)
			}
			server.Unlock()
		}
		hm.leaf.RUnlock()
		hm.processFailedNodes(failedServers)
		time.Sleep(2 * hm.healthyNodeTimeOut)
	}
}

func (hm *LeafHealthManager) Start() {
	go hm.processNodes()
	<-hm.doneChan
}
