package sequencer

import "github.com/khaledmdiab/horus_controller/core"

type jobScheduler struct {
}

func newJobScheduler() *jobScheduler {
	js := &jobScheduler{}
	return js
}

func (js *jobScheduler) schedule(sr *core.SyncJob) int {
	return 0
}
