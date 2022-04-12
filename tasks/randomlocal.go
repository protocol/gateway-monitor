package tasks

import (
	"context"
	"fmt"
	"strconv"

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
			Namespace:   "gatewaymonitor_task",
			Subsystem:   "random_local",
			Name:        "latency_seconds",
			Buckets:     prometheus.LinearBuckets(0, 6, 10), // 0-1 minutes
			ConstLabels: map[string]string{"size": strconv.Itoa(size)},
		},
		[]string{"pop"},
	)
	fetch_time := prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace:   "gatewaymonitor_task",
			Subsystem:   "random_local",
			Name:        "fetch_seconds",
			Buckets:     prometheus.LinearBuckets(0, 6, 15), // 0-1:30 minutes
			ConstLabels: map[string]string{"size": strconv.Itoa(size)},
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
	return &RandomLocalBench{
		reg:        &reg,
		size:       size,
		latency:    latency,
		fetch_time: fetch_time,
	}
}

func (t *RandomLocalBench) Run(ctx context.Context, sh *shell.Shell, ps *pinning.Client, gw string) error {
	defer gc(ctx, sh)
	localLabels := prometheus.Labels{"test": "random_local", "size": strconv.Itoa(t.size), "pop": "localhost"}

	cidstr, randb, err := addRandomData(sh, "random_local", t.size)
	if err != nil {
		return err
	}

	defer func() {
		log.Info("cleaning up IPFS node")
		err := sh.Unpin(cidstr)
		if err != nil {
			log.Warnw("failed to clean unpin cid.", "cid", cidstr)
			errors.With(localLabels).Inc()
		}
	}()

	// request from gateway, observing client metrics
	url := fmt.Sprintf("%s/ipfs/%s", gw, cidstr)

	return checkAndRecord(ctx, "random_local", gw, url, randb, t.latency, t.fetch_time)
}

func (t *RandomLocalBench) Registration() *task.Registration {
	return t.reg
}
