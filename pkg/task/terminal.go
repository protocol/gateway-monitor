package task

import (
	"context"

	shell "github.com/ipfs/go-ipfs-api"
	pinning "github.com/ipfs/go-pinning-service-http-client"
	"github.com/prometheus/client_golang/prometheus"
)

type TerminalTask struct {
	Done chan bool
}

func (t *TerminalTask) Name() string {
	return "terminal_task"
}

func (t *TerminalTask) LatencyHist() *prometheus.HistogramVec {
	return nil
}

func (t *TerminalTask) FetchHist() *prometheus.HistogramVec {
	return nil
}

func (t *TerminalTask) Run(context.Context, *shell.Shell, *pinning.Client, string) error {
	t.Done <- true
	return nil
}

func (t *TerminalTask) Registration() *Registration {
	return new(Registration)
}
