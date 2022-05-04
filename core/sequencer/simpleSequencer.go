package sequencer

import (
	"math"
	"sync"

	"github.com/khaledmdiab/horus_controller/core"
)

// SimpleEventSequencer ...
type SimpleEventSequencer struct {
	pendingJobs *pendingJobsMap
	seqIDMap    *sequenceMap

	statsLock         sync.RWMutex
	receivedJobCount  int
	processedJobCount int
	queuedJobCount    int

	hm *core.NodeHealthManager

	ingressChan chan *core.SyncJob
	egressChan  chan *core.SyncJobResult
	done        chan bool
}

// SimpleEventSequencerOption ...
type SimpleEventSequencerOption func(*SimpleEventSequencer)

// NewSimpleEventSequencer ...
func NewSimpleEventSequencer(ingressChan chan *core.SyncJob, egressChan chan *core.SyncJobResult,
	hm *core.NodeHealthManager,
	opts ...SimpleEventSequencerOption) *SimpleEventSequencer {
	es := &SimpleEventSequencer{
		ingressChan: ingressChan,
		egressChan:  egressChan,
		done:        make(chan bool),
		hm:          hm,
		pendingJobs: newPendingJobsMap(),
		seqIDMap:    newSequenceMap()}

	for _, opt := range opts {
		opt(es)
	}

	es.init()

	return es
}

func (es *SimpleEventSequencer) init() {
	es.statsLock.Lock()
	es.receivedJobCount = 0
	es.processedJobCount = 0
	es.statsLock.Unlock()
}

// Start ...
func (es *SimpleEventSequencer) Start() {
	go es.recv()
	go es.processPendingJobs()
	<-es.done
}

// Stop ...
func (es *SimpleEventSequencer) Stop() {
	es.done <- true
}

// GetSeqID ...
func (es *SimpleEventSequencer) GetSeqID(sessionID string) uint32 {
	return es.seqIDMap.Load(sessionID)
}

//
func (es *SimpleEventSequencer) recv() {
	for {
		select {
		case sr := <-es.ingressChan:
			es.statsLock.Lock()
			es.receivedJobCount++
			es.statsLock.Unlock()
			srAddress := string(sr.SessionAddress)
			oldSeqID := es.GetSeqID(srAddress)

			// A message from host to centralized controller
			if sr.SequenceID == math.MaxUint32 {
				sr.SequenceID = oldSeqID + 1
			}

			// Process "new" events only
			if sr.SequenceID >= oldSeqID {
				es.seqIDMap.Store(srAddress, sr.SequenceID)
			} else {
				// Ignore "old" events!
				continue
			}

			if sr.JobType == core.Sync {
				// If the address exists --> replace it with the new one
				// If the address *does not* exist --> add a new job
				es.pendingJobs.Store(srAddress, sr)
				// To Downstream
				result := core.NewSyncJobResult(sr.SessionAddress, core.Sync, sr.Receivers, sr.MsgContent, sr.SequenceID, 0)
				es.egressChan <- result
			} else if sr.JobType == core.Done {
				// check pending jobs
				if pendingJob, found := es.pendingJobs.Load(srAddress); found {
					pendingJob.OnDoneReceived(sr.Sender)
					if pendingJob.IsDone() {
						pjAddress := string(pendingJob.SessionAddress)
						es.pendingJobs.Delete(pjAddress)

						// To Upstream
						result := core.NewSyncJobResult(pendingJob.SessionAddress, core.Done, []string{}, []byte{}, sr.SequenceID, 0)
						es.egressChan <- result
					}
				}
			}
		default:
			continue
		}
	}
}

func (es *SimpleEventSequencer) processPendingJobs() {
	// TODO handles downstream failures (if a downstream has failed while it's in the pending jobs)
	//for {
	//	es.statsLock.RLock()
	//	log.Println("Received Job Count: ", es.receivedJobCount)
	//	log.Println("Processed Job Count: ", es.processedJobCount)
	//	es.statsLock.RUnlock()
	//	log.Println("Pending Job Count: ", es.pendingJobs.Size())
	//	time.Sleep(time.Second)
	//}
	select {}
}
