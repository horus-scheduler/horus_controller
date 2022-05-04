package core

import "sync"

type SyncJobType int

const (
	Sync SyncJobType = iota
	Done
)

type SyncJob struct {
	lock sync.RWMutex

	JobType         SyncJobType
	SessionAddress  []byte
	Sender          string   // who sends this job?
	Receivers       []string // who should receive the results?
	actualReceivers []string // who "actually" received the results so far?
	MsgContent      []byte
	SequenceID      uint32
	TimeIndex       float64
}

// NewSyncJob ...
func NewSyncJob(sessionAddress []byte, jobType SyncJobType,
	sender string, receivers []string, msgContent []byte,
	sequenceID uint32, timeIndex float64) *SyncJob {
	sr := &SyncJob{
		SessionAddress: sessionAddress,
		JobType:        jobType,
		Sender:         sender,
		Receivers:      receivers,
		MsgContent:     msgContent,
		SequenceID:     sequenceID,
		TimeIndex:      timeIndex}
	return sr
}

// OnDoneReceived ...
func (sr *SyncJob) OnDoneReceived(senderAddress string) {
	sr.lock.Lock()
	defer sr.lock.Unlock()

	sr.actualReceivers = append(sr.actualReceivers, senderAddress)
}

// IsDone ...
func (sr *SyncJob) IsDone() bool {
	sr.lock.RLock()
	actualCount := len(sr.actualReceivers)
	sr.lock.RUnlock()

	// TODO should check the content as well, not just the count!
	expectedCount := len(sr.Receivers)
	return actualCount == expectedCount
}
