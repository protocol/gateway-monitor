package tasks

import (
	"context"
	"fmt"

	"github.com/prometheus/client_golang/prometheus"

	shell "github.com/ipfs/go-ipfs-api"
	pinning "github.com/ipfs/go-pinning-service-http-client"

	"github.com/ipfs-shipyard/gateway-monitor/pkg/task"
)

type KnownGoodCheck struct {
	reg        *task.Registration
	checks     map[string][]byte
	latency    *prometheus.HistogramVec
	fetch_time *prometheus.HistogramVec
}

func NewKnownGoodCheck(schedule string, checks map[string][]byte) *KnownGoodCheck {
	latency := prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: "gatewaymonitor_task",
			Subsystem: "known_good",
			Name:      "latency_seconds",
			Buckets:   prometheus.LinearBuckets(0, 0.2, 10), // 0-2 seconds
		},
		[]string{"pop"},
	)
	fetch_time := prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: "gatewaymonitor_task",
			Subsystem: "known_good",
			Name:      "fetch_seconds",
			Buckets:   prometheus.LinearBuckets(0, 0.2, 10), // 0-2 seconds (small file)
		},
		[]string{"pop"},
	)
	reg := task.Registration{
		Schedule: schedule,
		Collectors: []prometheus.Collector{
			latency,
			fetch_time,
		},
	}
	return &KnownGoodCheck{
		reg:        &reg,
		checks:     checks,
		latency:    latency,
		fetch_time: fetch_time,
	}
}

func (t *KnownGoodCheck) Run(ctx context.Context, sh *shell.Shell, ps *pinning.Client, gw string) error {
	for ipfspath, value := range t.checks {
		// request from gateway, observing client metrics
		url := fmt.Sprintf("%s%s", gw, ipfspath)
		err := checkAndRecord(ctx, "known_good", gw, url, value, t.latency, t.fetch_time)
		if err != nil {
			return err
		}
	}
	return nil
}

func (t *KnownGoodCheck) Registration() *task.Registration {
	return t.reg
}
