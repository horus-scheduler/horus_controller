package core

import (
	"sync"

	"github.com/khaledmdiab/horus_controller/core/mc_algorithm"
	"github.com/khaledmdiab/horus_controller/core/model"
)

type sessionMap struct {
	sync.RWMutex
	internal map[string]*model.Session
}

func newSessionMap() *sessionMap {
	return &sessionMap{
		internal: make(map[string]*model.Session),
	}
}

func (rm *sessionMap) Load(key string) (value *model.Session, ok bool) {
	rm.RLock()
	result, ok := rm.internal[key]
	rm.RUnlock()
	return result, ok
}

func (rm *sessionMap) Delete(key string) {
	rm.Lock()
	delete(rm.internal, key)
	rm.Unlock()
}

func (rm *sessionMap) Store(key string, value *model.Session) {
	rm.Lock()
	rm.internal[key] = value
	rm.Unlock()
}

type SessionManager struct {
	sessions  *sessionMap
	algorithm mc_algorithm.MulticastAlgorithm
}

func NewSessionManager(algorithm mc_algorithm.MulticastAlgorithm) *SessionManager {
	return &SessionManager{
		sessions:  newSessionMap(),
		algorithm: algorithm,
		//doneChan: make(chan bool),
	}
}

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

func (sm *SessionManager) CreateSession(session *model.Session) *model.MulticastTree {
	sm.sessions.Store(session.SessionAddressString, session)
	return sm.algorithm.OnSessionCreated(session)
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
