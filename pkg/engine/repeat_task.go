package engine

import (
	"context"

	"github.com/ipfs-shipyard/gateway-monitor/pkg/task"
	shell "github.com/ipfs/go-ipfs-api"
	pinning "github.com/ipfs/go-pinning-service-http-client"
)

// RepeatTask is a Task that will repeat all the given tasks until a condition is met. The condition
// is evaluated at the beginning of each loop
// It's in the engine package instead of the task package to avoid a circular dependency between
// them (since engine depends on task)
type RepeatTask struct {
	engine *Engine
	tasks  []task.Task
	until  func() bool
}

func (e *Engine) NewRepeatTask(tasks []task.Task, until func() bool) *RepeatTask {
	return &RepeatTask{
		engine: e,
		tasks:  tasks,
		until:  until,
	}
}

func (e *Engine) RepeatForever(tasks []task.Task) *RepeatTask {
	return e.NewRepeatTask(tasks, func() bool { return false })
}

func (t *RepeatTask) Run(context.Context, *shell.Shell, *pinning.Client, string) error {
	if t.until() {
		log.Info("Loop finished")
		t.engine.AddTask(t.engine.TerminalTask())
		return nil
	}

	for _, tsk := range t.tasks {
		t.engine.AddTask(tsk)
	}

	t.engine.AddTask(t)
	log.Info("Looping")

	return nil
}

func (t *RepeatTask) Registration() *task.Registration {
	return new(task.Registration)
}
