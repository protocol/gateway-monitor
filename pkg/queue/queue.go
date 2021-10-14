package queue

import (
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/ipfs-shipyard/gateway-monitor/pkg/task"
)

var (
	queue_len = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Namespace: "gatewaymonitor",
			Subsystem: "queue",
			Name:      "length",
		})
	queue_fails = prometheus.NewCounter(
		prometheus.CounterOpts{
			Namespace: "gatewaymonitor",
			Subsystem: "queue",
			Name:      "fails",
		})
)

func init() {
	prometheus.Register(queue_len)
	prometheus.Register(queue_fails)
}

type TaskQueue struct {
	mu      sync.Mutex
	tasks   []task.Task
	taskmap map[task.Task]bool
}

func NewTaskQueue() *TaskQueue {
	return &TaskQueue{
		tasks:   []task.Task{},
		taskmap: make(map[task.Task]bool),
	}
}

func (q *TaskQueue) Len() int {
	return len(q.tasks)
}

func (q *TaskQueue) Push(tsks ...task.Task) {
	q.mu.Lock()
	defer q.mu.Unlock()

	for _, newtsk := range tsks {
		if _, found := q.taskmap[newtsk]; found {
			queue_fails.Inc()
			continue
		}
		q.tasks = append(q.tasks, newtsk)
		q.taskmap[newtsk] = true
		queue_len.Inc()
	}
}

func (q *TaskQueue) Pop() (task.Task, bool) {
	q.mu.Lock()
	defer q.mu.Unlock()

	if q.Len() == 0 {
		return nil, false
	}
	t := q.tasks[0]
	q.tasks = q.tasks[1:]
	delete(q.taskmap, t)
	queue_len.Dec()
	return t, true
}

func (q *TaskQueue) Subscribe() chan task.Task {
	ch := make(chan task.Task)
	go func() {
		ticker := time.NewTicker(500 * time.Millisecond)
		for {
			select {
			case <-ticker.C:
				t, ok := q.Pop()
				if ok {
					ch <- t
				}
			}
		}
	}()
	return ch
}
