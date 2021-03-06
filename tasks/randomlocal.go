package tasks

import (
	"context"
	"fmt"

	"github.com/prometheus/client_golang/prometheus"

	shell "github.com/ipfs/go-ipfs-api"
	pinning "github.com/ipfs/go-pinning-service-http-client"

	"github.com/ipfs-shipyard/gateway-monitor/pkg/task"
)

type RandomLocalBench struct {
	reg        *task.Registration
	size       int
	latency    *prometheus.HistogramVec
	fetch_time *prometheus.HistogramVec
}

func NewRandomLocalBench(schedule string, size int) *RandomLocalBench {
	latency := prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: "gatewaymonitor_task",
			Subsystem: "random_local",
			Name:      "latency_seconds",
			Buckets:   prometheus.LinearBuckets(0, 12, 11), // 0-2 minutes
		},
		defaultLabels,
	)
	fetch_time := prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: "gatewaymonitor_task",
			Subsystem: "random_local",
			Name:      "fetch_seconds",
			Buckets:   prometheus.LinearBuckets(0, 15, 16), // 0-4 minutes
		},
		defaultLabels,
	)
	reg := task.Registration{
		Schedule: schedule,
		Collectors: []prometheus.Collector{
			latency,
			fetch_time,
		},
	}
	return &RandomLocalBench{
		reg:        &reg,
		size:       size,
		latency:    latency,
		fetch_time: fetch_time,
	}
}

func (t *RandomLocalBench) Name() string {
	return "random_local"
}

func (t *RandomLocalBench) LatencyHist() *prometheus.HistogramVec {
	return t.latency
}

func (t *RandomLocalBench) FetchHist() *prometheus.HistogramVec {
	return t.fetch_time
}

func (t *RandomLocalBench) Run(ctx context.Context, sh *shell.Shell, ps *pinning.Client, gw string) error {
	defer gc(ctx, sh)

	cidstr, randb, err := addRandomData(sh, t, t.size)
	if err != nil {
		return err
	}

	defer func() {
		localLabels := task.Labels(t, "localhost", t.size, 0)
		log.Info("Unpinning test CID")
		err := sh.Unpin(cidstr)
		if err != nil {
			log.Warnw("Failed to clean unpin cid.", "cid", cidstr)
			errors.With(localLabels).Inc()
		}
	}()

	// request from gateway, observing client metrics
	url := fmt.Sprintf("%s/ipfs/%s", gw, cidstr)

	return checkAndRecord(ctx, t, gw, url, randb)
}

func (t *RandomLocalBench) Registration() *task.Registration {
	return t.reg
}
