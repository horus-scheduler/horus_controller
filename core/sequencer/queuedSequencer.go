package sequencer

import (
	"sync"

	"github.com/khaledmdiab/horus_controller/core"
)

const DefaultQueueCount = 8
const DefaultQueueSize = 100

type EventSequencer struct {
	queues     []*jobQueue
	queueCount int
	queueSize  int

	jobScheduler *jobScheduler
	pendingJobs  *pendingJobsMap

	statsLock         sync.RWMutex
	receivedJobCount  int
	processedJobCount int
	queuedJobCount    int

	hm *core.NodeHealthManager

	ingressChan chan *core.SyncJob
	egressChan  chan *core.SyncJobResult
	done        chan bool
}

type EventSequencerOption func(*EventSequencer)

func NewEventSequencer(ingressChan chan *core.SyncJob, egressChan chan *core.SyncJobResult,
	hm *core.NodeHealthManager,
	opts ...EventSequencerOption) *EventSequencer {
	js := newJobScheduler()
	es := &EventSequencer{
		ingressChan:  ingressChan,
		egressChan:   egressChan,
		done:         make(chan bool),
		hm:           hm,
		jobScheduler: js,
		pendingJobs:  newPendingJobsMap(),
		queueCount:   DefaultQueueCount,
		queueSize:    DefaultQueueSize}

	for _, opt := range opts {
		opt(es)
	}

	es.init()

	return es
}

func WithQueueCount(queueCount int) EventSequencerOption {
	return func(es *EventSequencer) {
		es.queueCount = queueCount
	}
}

func WithQueueSize(queueSize int) EventSequencerOption {
	return func(es *EventSequencer) {
		es.queueSize = queueSize
	}
}

func (es *EventSequencer) init() {
	es.statsLock.Lock()
	es.receivedJobCount = 0
	es.processedJobCount = 0
	es.statsLock.Unlock()
	es.queues = make([]*jobQueue, es.queueCount)
	for q := 0; q < es.queueCount; q++ {
		es.queues[q] = newQueue(es.queueSize)
	}
}

//
func (es *EventSequencer) Start() {
	go es.recv()
	for qIdx := 0; qIdx < es.queueCount; qIdx++ {
		go es.processQueue(qIdx)
	}
	go es.processPendingJobs()
	<-es.done
}

func (es *EventSequencer) Stop() {
	es.done <- true
}

//
func (es *EventSequencer) recv() {
	for {
		select {
		case sr := <-es.ingressChan:
			es.statsLock.Lock()
			es.receivedJobCount += 1
			es.statsLock.Unlock()
			if sr.JobType == core.Sync {
				queueIndex := es.jobScheduler.schedule(sr)
				es.queues[queueIndex].push(sr)
			} else if sr.JobType == core.Done {
				// check pending jobs
				srAddress := string(sr.SessionAddress)
				if pendingJob, found := es.pendingJobs.Load(srAddress); found {
					pendingJob.OnDoneReceived(sr.Sender)
					if pendingJob.IsDone() {
						pjAddress := string(pendingJob.SessionAddress)
						es.pendingJobs.Delete(pjAddress)
					}
					// TODO if this is a ToR, we need to propagate the "Done" msg back to the controller
					// If it's the controller, do nothing; just remove the job
					result := core.NewSyncJobResult(pendingJob.SessionAddress, core.Done, []string{}, []byte{}, 0, 0)
					es.egressChan <- result
					es.statsLock.Lock()
					es.processedJobCount += 1
					es.statsLock.Unlock()
				}
			}
		default:
			continue
		}
	}
}

func (es *EventSequencer) processQueue(qIdx int) {
	queue := es.queues[qIdx]
	for {
		if !queue.empty() {
			job := queue.peek()
			jAddress := string(job.SessionAddress)
			if _, isPending := es.pendingJobs.Load(jAddress); !isPending {
				job := queue.pop()
				es.pendingJobs.Store(jAddress, job)

				// To host
				result := core.NewSyncJobResult(job.SessionAddress, core.Sync, job.Receivers, job.MsgContent, 0, 0)
				es.egressChan <- result
				es.statsLock.Lock()
				es.processedJobCount += 1
				es.statsLock.Unlock()
			}
		}
	}
}

func (es *EventSequencer) processPendingJobs() {
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
