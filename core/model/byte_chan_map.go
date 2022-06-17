package model

import "sync"

type ByteChanMap struct {
	sync.RWMutex
	internal map[uint16]chan []byte
}

func NewByteChanMap() *ByteChanMap {
	return &ByteChanMap{
		internal: make(map[uint16]chan []byte),
	}
}

func (rm *ByteChanMap) Internal() map[uint16]chan []byte {
	rm.Lock()
	defer rm.Unlock()
	return rm.internal
}

func (rm *ByteChanMap) Load(key uint16) (value chan []byte, ok bool) {
	rm.RLock()
	result, ok := rm.internal[key]
	rm.RUnlock()
	return result, ok
}

func (rm *ByteChanMap) Delete(key uint16) {
	rm.Lock()
	delete(rm.internal, key)
	rm.Unlock()
}

func (rm *ByteChanMap) Store(key uint16, value chan []byte) {
	rm.Lock()
	rm.internal[key] = value
	rm.Unlock()
}
