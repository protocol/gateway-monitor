package tasks

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/ipfs/go-cid"
	shell "github.com/ipfs/go-ipfs-api"
	pinning "github.com/ipfs/go-pinning-service-http-client"

	"github.com/ipfs-shipyard/gateway-monitor/pkg/task"
)

type RandomPinningBench struct {
	reg        *task.Registration
	size       int
	latency    *prometheus.HistogramVec
	fetch_time *prometheus.HistogramVec
}

func NewRandomPinningBench(schedule string, size int) *RandomPinningBench {
	latency := prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace:   "gatewaymonitor_task",
			Subsystem:   "random_pinning",
			Name:        "latency_seconds",
			Buckets:     prometheus.LinearBuckets(0, 12, 11), // 0-2 minutes
			ConstLabels: map[string]string{"size": strconv.Itoa(size)},
		},
		[]string{"pop", "code"},
	)
	fetch_time := prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace:   "gatewaymonitor_task",
			Subsystem:   "random_pinning",
			Name:        "fetch_seconds",
			Buckets:     prometheus.LinearBuckets(0, 15, 16), // 0-4 minutes
			ConstLabels: map[string]string{"size": strconv.Itoa(size)},
		},
		[]string{"pop", "code"},
	)
	reg := task.Registration{
		Schedule: schedule,
		Collectors: []prometheus.Collector{
			latency,
			fetch_time,
		},
	}
	return &RandomPinningBench{
		reg:        &reg,
		size:       size,
		latency:    latency,
		fetch_time: fetch_time,
	}
}

func (t *RandomPinningBench) Run(ctx context.Context, sh *shell.Shell, ps *pinning.Client, gw string) error {
	defer gc(ctx, sh)

	localLabels := prometheus.Labels{"test": "random_pinning", "size": strconv.Itoa(t.size), "pop": "localhost"}
	pinLabels := prometheus.Labels{"test": "random_pinning", "size": strconv.Itoa(t.size), "pop": "pinning"}

	cidstr, randb, err := addRandomData(sh, "random_pinning", t.size)
	if err != nil {
		return err
	}

	defer func() {
		log.Info("Unpinning test CID")
		// don't bother error checking. We clean it up explicitly in the happy path.
		sh.Unpin(cidstr)
	}()

	// Pin to pinning service
	c, err := cid.Decode(cidstr)
	if err != nil {
		errors.With(localLabels).Inc()
		return fmt.Errorf("failed to decode cid after it was returned from IPFS: %w", err)
	}
	getter, err := ps.Add(ctx, c)
	if err != nil {
		errors.With(pinLabels).Inc()
		return fmt.Errorf("failed to pin cid to pinning service: %w", err)
	}

	// long poll pinning service
	log.Info("waiting for pinning service to complete the pin")
	var pinned bool
	for !pinned {
		status, err := ps.GetStatusByID(ctx, getter.GetRequestId())
		if err == nil {
			fmt.Println(status.GetStatus())
			pinned = status.GetStatus() == pinning.StatusPinned
		} else {
			errors.With(pinLabels).Inc()
			fmt.Println(err)
		}
		time.Sleep(time.Minute)
	}

	// delete this from our local IPFS node.
	log.Info("removing pin from local IPFS node")
	err = sh.Unpin(cidstr)
	if err != nil {
		errors.With(localLabels).Inc()
		return fmt.Errorf("Could not unpin cid after adding it earlier: %w", err)
	}

	url := fmt.Sprintf("%s/ipfs/%s", gw, cidstr)
	return checkAndRecord(ctx, "random_pinning", gw, url, randb, t.latency, t.fetch_time)
}

func (t *RandomPinningBench) Registration() *task.Registration {
	return t.reg
}
