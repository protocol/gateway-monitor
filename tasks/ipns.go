package tasks

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"strconv"
	"time"

	"github.com/prometheus/client_golang/prometheus"

	shell "github.com/ipfs/go-ipfs-api"
	pinning "github.com/ipfs/go-pinning-service-http-client"

	"github.com/ipfs-shipyard/gateway-monitor/pkg/task"
)

type IpnsBench struct {
	reg          *task.Registration
	size         int
	publish_time prometheus.Histogram
	latency      *prometheus.HistogramVec
	fetch_time   *prometheus.HistogramVec
}

func NewIpnsBench(schedule string, size int) *IpnsBench {
	publish_time := prometheus.NewHistogram(
		prometheus.HistogramOpts{
			Namespace:   "gatewaymonitor_task",
			Subsystem:   "ipns",
			Name:        "publish_seconds",
			Buckets:     prometheus.LinearBuckets(0, 10, 10), // 0-10 seconds
			ConstLabels: map[string]string{"size": strconv.Itoa(size)},
		},
	)
	latency := prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace:   "gatewaymonitor_task",
			Subsystem:   "ipns",
			Name:        "latency_seconds",
			Buckets:     prometheus.LinearBuckets(0, 6, 10), // 0-1 minutes
			ConstLabels: map[string]string{"size": strconv.Itoa(size)},
		},
		[]string{"pop"},
	)
	fetch_time := prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace:   "gatewaymonitor_task",
			Subsystem:   "ipns",
			Name:        fmt.Sprintf("%d_fetch_seconds", size),
			Buckets:     prometheus.LinearBuckets(0, 6, 15), // 0-1:30 minutes
			ConstLabels: map[string]string{"size": strconv.Itoa(size)},
		},
		[]string{"pop"},
	)
	reg := task.Registration{
		Schedule: schedule,
		Collectors: []prometheus.Collector{
			publish_time,
			latency,
			fetch_time,
		},
	}
	return &IpnsBench{
		reg:          &reg,
		size:         size,
		publish_time: publish_time,
		latency:      latency,
		fetch_time:   fetch_time,
	}
}

func (t *IpnsBench) Run(ctx context.Context, sh *shell.Shell, ps *pinning.Client, gw string) error {
	defer gc(ctx, sh)

	localLabels := prometheus.Labels{"test": "ipns", "size": strconv.Itoa(t.size), "pop": "localhost"}

	// generate random data
	log.Infof("generating %d bytes random data", t.size)
	randb := make([]byte, t.size)
	if _, err := rand.Read(randb); err != nil {
		errors.With(localLabels).Inc()
		return fmt.Errorf("failed to generate random values: %w", err)
	}
	buf := bytes.NewReader(randb)

	// add to local ipfs
	log.Info("writing data to local IPFS node")
	cidstr, err := sh.Add(buf)
	if err != nil {
		log.Errorw("failed to write to IPFS", "err", err)
		errors.With(localLabels).Inc()
		return err
	}
	defer func() {
		log.Info("cleaning up IPFS node")
		err := sh.Unpin(cidstr)
		if err != nil {
			errors.With(localLabels).Inc()
			log.Warnw("failed to clean unpin cid.", "cid", cidstr)
		}
	}()

	// Generate a new key
	// we already have a random value lying around, might as
	// well use it for the ney name.
	keyName := base64.StdEncoding.EncodeToString(randb[:8])
	_, err = sh.KeyGen(ctx, keyName)
	if err != nil {
		errors.With(localLabels).Inc()
		return fmt.Errorf("failed to generate new key: %w", err)
	}
	defer func() {
		sh.KeyRm(ctx, keyName)
	}()

	// Publish IPNS
	pub_start := time.Now()
	pubResp, err := sh.PublishWithDetails(cidstr, keyName, time.Hour, time.Hour, true)
	publish_time := time.Since(pub_start).Seconds()
	log.Infow("published IPNS", "seconds", publish_time, "cid", cidstr, "ipns", pubResp.Name)
	t.publish_time.Observe(float64(publish_time))

	// request from gateway, observing client metrics
	url := fmt.Sprintf("%s/ipns/%s", gw, pubResp.Name)
	return checkAndRecord(ctx, "ipns", gw, url, randb, t.latency, t.fetch_time)
}

func (t *IpnsBench) Registration() *task.Registration {
	return t.reg
}
