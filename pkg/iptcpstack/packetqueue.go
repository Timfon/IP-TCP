package iptcpstack

import (
	"container/heap"
)

// An Item is something we manage in a priority queue.
type Item struct {
	value []byte


	priority uint32   // The priority of the item in the queue.
	// The index is needed by update and is maintained by the heap.Interface methods.
	index int // The index of the item in the heap.
}

type PriorityQueue []*Item

func (pq PriorityQueue) Len() int { return len(pq) }

func (pq PriorityQueue) Less(i, j int) bool {
	// We want Pop to give us the highest, not lowest, priority so we use greater than here.
	return pq[i].priority < pq[j].priority
}

func (pq PriorityQueue) Swap(i, j int) {
	pq[i], pq[j] = pq[j], pq[i]
	pq[i].index = i
	pq[j].index = j
}

func (pq *PriorityQueue) Push(x any) {
	n := len(*pq)
	item := x.(*Item)
	item.index = n
	*pq = append(*pq, item)
}

func (pq *PriorityQueue) Pop() any {
	old := *pq
	n := len(old)
	item := old[n-1]
	old[n-1] = nil  // don't stop the GC from reclaiming the item eventually
	item.index = -1 // for safety
	*pq = old[0 : n-1]
	return item
}
func (pq PriorityQueue) Peek() interface{} {
    if pq.Len() == 0 {
        return nil
    }
    return pq[0]
}
// update modifies the priority and value of an Item in the queue.
func (pq *PriorityQueue) update(item *Item, v []byte, priority uint32) {
	item.value = v
	item.priority = priority
	heap.Fix(pq, item.index)
}
