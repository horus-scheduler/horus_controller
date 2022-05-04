package sequencer

import (
	"sync"

	"github.com/khaledmdiab/horus_controller/core"
)

// NewQueue returns a new queue with the given initial size.
func newQueue(size int) *jobQueue {
	return &jobQueue{
		nodes: make([]*core.SyncJob, size),
		size:  size,
	}
}

// Queue is a basic FIFO queue based on a circular list that resizes as needed.
type jobQueue struct {
	sync.RWMutex

	nodes []*core.SyncJob
	size  int
	head  int
	tail  int
	count int
}

// Push adds a node to the queue.
func (q *jobQueue) push(request *core.SyncJob) {
	q.Lock()
	defer q.Unlock()

	if q.head == q.tail && q.count > 0 {
		nodes := make([]*core.SyncJob, len(q.nodes)+q.size)
		copy(nodes, q.nodes[q.head:])
		copy(nodes[len(q.nodes)-q.head:], q.nodes[:q.head])
		q.head = 0
		q.tail = len(q.nodes)
		q.nodes = nodes
	}
	q.nodes[q.tail] = request
	q.tail = (q.tail + 1) % len(q.nodes)
	q.count++
}

// Pop removes and returns a node from the queue in first to last order.
func (q *jobQueue) pop() *core.SyncJob {
	if q.empty() {
		return nil
	}

	q.Lock()
	defer q.Unlock()
	node := q.nodes[q.head]
	q.head = (q.head + 1) % len(q.nodes)
	q.count--
	return node
}

func (q *jobQueue) cnt() int {
	q.RLock()
	c := q.count
	q.RUnlock()
	return c
}

// Peek returns a node from the queue in first to last order.
func (q *jobQueue) peek() *core.SyncJob {
	if q.empty() {
		return nil
	}

	q.RLock()
	defer q.RUnlock()
	node := q.nodes[q.head]
	return node
}

func (q *jobQueue) sortPreservingOrder(k int, m map[string]*core.SyncJob) {
	return
	//if q.count == 0 {
	//	return
	//}
	//
	//headItem := q.nodes[q.head]
	//if _, ok := m[headItem.address]; !ok  {
	//	return
	//}
	//
	//if m[headItem.address].done {
	//	return
	//}
	//
	//maxIndex := min(q.head + k, q.tail)
	//finalIndex := -1
	//
	//for j := q.head+1; j <= maxIndex; j++ {
	//	item := q.nodes[j]
	//	_, ok := m[item.address]
	//	if !ok || (ok && m[item.address].done) {
	//		finalIndex = j
	//		break
	//	}
	//
	//}

}

func (q *jobQueue) empty() bool {
	q.RLock()
	defer q.RUnlock()

	return q.count == 0
}
