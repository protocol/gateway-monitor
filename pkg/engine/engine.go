package engine

import (
	"context"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/robfig/cron/v3"

	logging "github.com/ipfs/go-log"

	shell "github.com/ipfs/go-ipfs-api"
	pinning "github.com/ipfs/go-pinning-service-http-client"

	"github.com/ipfs-shipyard/gateway-monitor/pkg/queue"
	"github.com/ipfs-shipyard/gateway-monitor/pkg/task"
)

var log = logging.Logger("engine")

type Engine struct {
	c    *cron.Cron
	q    *queue.TaskQueue
	sh   *shell.Shell
	ps   *pinning.Client
	gw   string
	done chan bool
}

// Create an engine with Cron and Prometheus setup
func New(sh *shell.Shell, ps *pinning.Client, gw string, tsks ...task.Task) *Engine {
	q := queue.New()
	c := cron.New()
	return NewWithQueueAndCron(q, c, sh, ps, gw, tsks...)
}

// Create an engine passing a queue and cron instance
// If we ever decide that parallelism is desirable, multiple engines could
// subscribe to the same queue. In that case, you would instantiate one engine with tasks
// so they are registered once. Then, any subsequent engines with which the queue is shared
// will run over the same tasks in parallel.
func NewWithQueueAndCron(q *queue.TaskQueue, c *cron.Cron, sh *shell.Shell, ps *pinning.Client, gw string, tsks ...task.Task) *Engine {
	eng := Engine{
		c:    c,
		q:    q,
		sh:   sh,
		ps:   ps,
		gw:   gw,
		done: make(chan bool),
	}

	for _, t := range tsks {
		reg := t.Registration()
		eng.c.AddFunc(reg.Schedule, scheduleClosure(eng.q, t))
		for _, col := range reg.Collectors {
			prometheus.Register(col)
		}
	}
	eng.c.Start()
	return &eng
}

// Create an engine without Cron and prometheus.
func NewSingle(sh *shell.Shell, ps *pinning.Client, gw string, tsks ...task.Task) *Engine {
	eng := Engine{
		c:    cron.New(),
		q:    queue.New(),
		sh:   sh,
		ps:   ps,
		gw:   gw,
		done: make(chan bool, 1),
	}

	for _, t := range tsks {
		for _, col := range t.Registration().Collectors {
			prometheus.Register(col)
		}
		eng.q.Push(t)
	}

	return &eng
}

func (e *Engine) Start(ctx context.Context) chan error {
	errCh := make(chan error)

	go func() {
		defer close(errCh)
		tch := e.q.Subscribe()
		for {
			select {
			case t := <-tch:
				c, cancel := context.WithTimeout(ctx, 10*time.Minute)
				defer cancel()
				log.Infof("Starting task %w", t)
				if err := t.Run(c, e.sh, e.ps, e.gw); err != nil {
					errCh <- err
				}
				log.Infof("Finished task %w", t)
			case <-e.done:
				return
			}
		}
	}()

	return errCh
}

func (e *Engine) Stop() {
	e.done <- true
}

func (e *Engine) AddTask(t task.Task) {
	e.q.Push(t)
}

func (e *Engine) TerminalTask() task.Task {
	return &task.TerminalTask{
		Done: e.done,
	}
}

func scheduleClosure(q *queue.TaskQueue, t task.Task) func() {
	return func() {
		q.Push(t)
	}
}
