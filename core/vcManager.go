package core

import (
	"sync"

	"github.com/khaledmdiab/horus_controller/core/model"
)

type vcMap struct {
	sync.RWMutex
	internal map[uint16]*model.VirtualCluster
}

func newVCMap() *vcMap {
	return &vcMap{
		internal: make(map[uint16]*model.VirtualCluster),
	}
}

func (rm *vcMap) Load(key uint16) (value *model.VirtualCluster, ok bool) {
	rm.RLock()
	result, ok := rm.internal[key]
	rm.RUnlock()
	return result, ok
}

func (rm *vcMap) Delete(key uint16) {
	rm.Lock()
	delete(rm.internal, key)
	rm.Unlock()
}

func (rm *vcMap) Store(key uint16, value *model.VirtualCluster) {
	rm.Lock()
	rm.internal[key] = value
	rm.Unlock()
}

type VCManager struct {
	vcs      *vcMap
	topology *model.SimpleTopology
}

func NewVCManager(topology *model.SimpleTopology) *VCManager {
	return &VCManager{
		vcs:      newVCMap(),
		topology: topology,
	}
}

func (vcm *VCManager) AddVC(vc *model.VirtualCluster) {
	vcm.vcs.Store(vc.ClusterID, vc)
}

// TODO: get which VCs are impacted by a server failure
// Keep track of:
// map[serverID] --> list of VCs
// map[leafID] --> list of VCs

/*
func (sm *SessionManager) ActivateSession(sessionAddress string) *model.MulticastTree {
	if s, found := sm.sessions.Load(sessionAddress); found {
		s.Activate()
		return sm.algorithm.OnSessionActivated(s)
	}
	return nil
}

func (sm *SessionManager) DeactivateSession(sessionAddress string) *model.MulticastTree {
	if s, found := sm.sessions.Load(sessionAddress); found {
		s.Deactivate()
		return sm.algorithm.OnSessionDeactivated(s)
	}
	return nil
}


func (sm *SessionManager) AddReceiver(sessionAddress string, receiver *model.Node) *model.MulticastTree {
	if s, found := sm.sessions.Load(sessionAddress); found {
		s.AddReceiver(receiver)
		return sm.algorithm.OnReceiverAdded(s, receiver)
	}
	return nil
}

func (sm *SessionManager) RemoveReceiver(sessionAddress string, receiver *model.Node) *model.MulticastTree {
	if s, found := sm.sessions.Load(sessionAddress); found {
		s.RemoveReceiver(receiver)
		return sm.algorithm.OnReceiverRemoved(s, receiver)
	}
	return nil
}

func (sm *SessionManager) RemoveReceiverByAddress(sessionAddress string, receiverAddress string) *model.MulticastTree {
	if s, found := sm.sessions.Load(sessionAddress); found {
		s.RemoveReceiverByAddress(receiverAddress)
		return sm.algorithm.OnReceiverRemovedByAddress(s, receiverAddress)
	}
	return nil
}
*/
