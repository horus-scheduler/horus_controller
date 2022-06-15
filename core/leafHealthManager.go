package core

import (
	"errors"
	"strconv"
	"time"

	"github.com/khaledmdiab/horus_controller/core/model"
	"github.com/sirupsen/logrus"
)

// Tracks the health of its servers
type LeafHealthManager struct {
	hmEgress           chan *LeafHealthMsg
	leaf               *model.Node
	topology           *model.Topology
	vcMgr              *VCManager
	healthyNodeTimeOut time.Duration
	DoneChan           chan bool
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
		DoneChan:           make(chan bool),
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
		_, us := hm.topology.RemoveServer(server.ID)
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
	stop := false
	for {
		if stop {
			break
		}
		select {
		case <-hm.DoneChan:
			logrus.Debug("[LeafHealthMgr] Shutting down the health manager")
			stop = true
		default:
			var failedServers []*model.Node
			hm.leaf.RLock()
			for _, server := range hm.leaf.Children {
				server.Lock()
				if hm.leaf.ID == 0 {
					logrus.Tracef("[LeafHealthMgr] Server %d, Leaf %d", server.ID, hm.leaf.ID)
				}
				elapsedTime := time.Since(server.LastPingTime)
				if elapsedTime >= hm.healthyNodeTimeOut {
					if server.Healthy && server.Ready {
						failedServers = append(failedServers, server)
						server.Healthy = false
						server.Ready = false
					}
				}
				server.Unlock()
			}
			hm.leaf.RUnlock()
			hm.processFailedNodes(failedServers)
			time.Sleep(2 * hm.healthyNodeTimeOut)
		}
	}
}

func (hm *LeafHealthManager) Start() {
	go hm.processNodes()
}
