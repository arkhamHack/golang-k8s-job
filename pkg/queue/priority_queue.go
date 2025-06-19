package queue

import (
	"container/heap"
	"sync"
)

type Job struct {
	ID       string      `json:"id"`
	Name     string      `json:"name"`
	Priority int         `json:"priority"`
	Spec     interface{} `json:"spec"`
}

type MaxPriorityQueue struct {
	items []*Job
	mu    sync.RWMutex
}

func (pq *MaxPriorityQueue) Len() int {
	pq.mu.RLock()
	defer pq.mu.RUnlock()
	return len(pq.items)
}

func (pq *MaxPriorityQueue) Less(i, j int) bool {
	return pq.items[i].Priority > pq.items[j].Priority
}

func (pq *MaxPriorityQueue) Swap(i, j int) {
	pq.items[i], pq.items[j] = pq.items[j], pq.items[i]
}

func (pq *MaxPriorityQueue) Push(x interface{}) {
	pq.mu.Lock()
	defer pq.mu.Unlock()
	item := x.(*Job)
	pq.items = append(pq.items, item)
}

func (pq *MaxPriorityQueue) Pop() interface{} {
	pq.mu.Lock()
	defer pq.mu.Unlock()
	old := pq.items
	n := len(old)
	if n == 0 {
		return nil
	}
	item := old[n-1]
	pq.items = old[0 : n-1]
	return item
}

func (pq *MaxPriorityQueue) Peek() *Job {
	pq.mu.RLock()
	defer pq.mu.RUnlock()
	if len(pq.items) == 0 {
		return nil
	}
	return pq.items[0]
}

func (pq *MaxPriorityQueue) PendingJobs() []*Job {
	pq.mu.RLock()
	defer pq.mu.RUnlock()

	result := make([]*Job, len(pq.items))
	copy(result, pq.items)
	return result
}

func NewPriorityQueue() *MaxPriorityQueue {
	pq := &MaxPriorityQueue{
		items: make([]*Job, 0),
	}
	heap.Init(pq)
	return pq
}
