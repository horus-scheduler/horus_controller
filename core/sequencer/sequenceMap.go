package sequencer

import (
	"sync"
)

type sequenceMap struct {
	sync.RWMutex
	internal map[string]uint32
}

func newSequenceMap() *sequenceMap {
	return &sequenceMap{
		internal: make(map[string]uint32),
	}
}

func (sm *sequenceMap) Load(key string) uint32 {
	sm.Lock()
	defer sm.Unlock()

	var ok bool
	var result uint32
	if result, ok = sm.internal[key]; !ok {
		result = 0
		sm.internal[key] = result
	}
	return result
}

func (sm *sequenceMap) Size() int {
	sm.RLock()
	size := len(sm.internal)
	sm.RUnlock()
	return size
}

func (sm *sequenceMap) Delete(key string) {
	sm.Lock()
	delete(sm.internal, key)
	sm.Unlock()
}

func (sm *sequenceMap) Store(key string, value uint32) {
	sm.Lock()
	sm.internal[key] = value
	sm.Unlock()
}
