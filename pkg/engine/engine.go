package engine

import (
	"context"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/robfig/cron"

	shell "github.com/ipfs/go-ipfs-api"
	pinning "github.com/ipfs/go-pinning-service-http-client"

	"github.com/ipfs-shipyard/gateway-monitor/pkg/queue"
	"github.com/ipfs-shipyard/gateway-monitor/pkg/task"
)

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
	q := queue.NewTaskQueue()
	return NewWithQueue(q, sh, ps, gw, tsks...)
}

func NewWithQueue(q *queue.TaskQueue, sh *shell.Shell, ps *pinning.Client, gw string, tsks ...task.Task) *Engine {
	eng := Engine{
		c:    cron.New(),
		q:    queue.NewTaskQueue(),
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
		q:    queue.NewTaskQueue(),
		sh:   sh,
		ps:   ps,
		gw:   gw,
		done: make(chan bool, 1),
	}

	for _, t := range tsks {
		eng.q.Push(t)
	}
	eng.q.Push(
		&task.TerminalTask{
			Done: eng.done,
		})
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
				if err := t.Run(c, e.sh, e.ps, e.gw); err != nil {
					errCh <- err
				}
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

func scheduleClosure(q *queue.TaskQueue, t task.Task) func() {
	return func() {
		q.Push(t)
	}
}
