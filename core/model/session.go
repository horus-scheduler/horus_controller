package model

import (
	"sync"
)

type Session struct {
	sync.RWMutex
	SessionAddress       []byte
	SessionAddressString string
	Active               bool
	Sender               *Node
	Receivers            []*Node
	receiverMap          map[string]*Node // receiverAddress -> Host
	receiverIdxMap       map[string]int   // receiverAddress -> index of Host in Receivers
}

func NewSession(addr []byte, active bool, sender *Node) *Session {
	s := &Session{SessionAddress: addr,
		SessionAddressString: string(addr),
		Active:               active,
		Sender:               sender,
	}
	s.receiverMap = make(map[string]*Node)
	s.receiverIdxMap = make(map[string]int)

	return s
}

func (s *Session) Activate() {
	s.Lock()
	defer s.Unlock()
	s.Active = true
}

func (s *Session) Deactivate() {
	s.Lock()
	defer s.Unlock()
	s.Active = false
}

func (s *Session) AddReceiver(receiver *Node) {
	s.Lock()
	defer s.Unlock()

	if _, found := s.receiverMap[receiver.Address]; !found {
		newIdx := len(s.Receivers)
		s.Receivers = append(s.Receivers, receiver)
		s.receiverIdxMap[receiver.Address] = newIdx
		s.receiverMap[receiver.Address] = receiver
	}
}

func (s *Session) RemoveReceiverByAddress(receiverAddress string) {
	s.Lock()
	defer s.Unlock()

	if _, found := s.receiverMap[receiverAddress]; found {
		removedRecIdx := s.receiverIdxMap[receiverAddress]
		lastRecIdx := len(s.Receivers) - 1
		lastRec := s.Receivers[lastRecIdx]

		s.receiverIdxMap[lastRec.Address] = removedRecIdx
		s.Receivers[removedRecIdx] = lastRec
		// We do not need to put s.Receivers[idx] at the end, as it will be discarded anyway
		s.Receivers = s.Receivers[:lastRecIdx]
		delete(s.receiverIdxMap, receiverAddress)
		delete(s.receiverMap, receiverAddress)
	}
}

func (s *Session) RemoveReceiver(receiver *Node) {
	s.RemoveReceiverByAddress(receiver.Address)
}
