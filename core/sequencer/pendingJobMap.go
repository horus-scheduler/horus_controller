package sequencer

import (
	"sync"

	"github.com/khaledmdiab/horus_controller/core"
)

type pendingJobsMap struct {
	sync.RWMutex
	internal map[string]*core.SyncJob
}

func newPendingJobsMap() *pendingJobsMap {
	return &pendingJobsMap{
		internal: make(map[string]*core.SyncJob),
	}
}

func (rm *pendingJobsMap) Load(key string) (value *core.SyncJob, ok bool) {
	rm.RLock()
	result, ok := rm.internal[key]
	rm.RUnlock()
	return result, ok
}

func (rm *pendingJobsMap) Size() int {
	rm.RLock()
	size := len(rm.internal)
	rm.RUnlock()
	return size
}

func (rm *pendingJobsMap) Delete(key string) {
	rm.Lock()
	delete(rm.internal, key)
	rm.Unlock()
}

func (rm *pendingJobsMap) Store(key string, value *core.SyncJob) {
	rm.Lock()
	rm.internal[key] = value
	rm.Unlock()
}
