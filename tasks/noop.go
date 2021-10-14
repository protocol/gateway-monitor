package tasks

import (
	"context"
	"fmt"
	"time"

	"github.com/prometheus/client_golang/prometheus"

	shell "github.com/ipfs/go-ipfs-api"
	pinning "github.com/ipfs/go-pinning-service-http-client"

	"github.com/ipfs-shipyard/gateway-monitor/pkg/task"
)

type NoopTask struct {
	schedule string
	i        int
	g        prometheus.Gauge
}

func (t *NoopTask) Run(ctx context.Context, sh *shell.Shell, ps *pinning.Client, gw string) error {
	for i := 0; i < t.i; i++ {
		time.Sleep(time.Second)
		fmt.Println("test")
		t.g.Add(1)
	}
	return nil
}

func (t *NoopTask) Registration() *task.Registration {
	return &task.Registration{
		Collectors: []prometheus.Collector{t.g},
		Schedule:   t.schedule,
	}
}

func NewNoopTask(schedule string, i int) *NoopTask {
	return &NoopTask{
		schedule: schedule,
		i:        i,
		g: prometheus.NewGauge(
			prometheus.GaugeOpts{
				Namespace: "gatewaymonitor_task",
				Subsystem: "noop",
				Name:      "noopgauge",
			},
		),
	}
}
