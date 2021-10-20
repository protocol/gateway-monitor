package tasks

import (
	"bytes"
	"context"
	"crypto/rand"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httptrace"
	"reflect"
	"time"

	"github.com/prometheus/client_golang/prometheus"

	shell "github.com/ipfs/go-ipfs-api"
	pinning "github.com/ipfs/go-pinning-service-http-client"

	"github.com/ipfs-shipyard/gateway-monitor/pkg/task"
)

type RandomLocalBench struct {
	reg        *task.Registration
	size       int
	start_time *prometheus.HistogramVec
	fetch_time *prometheus.HistogramVec
	fails      *prometheus.CounterVec
	errors     prometheus.Counter
}

func NewRandomLocalBench(schedule string, size int) *RandomLocalBench {
	start_time := prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: "gatewaymonitor_task",
			Subsystem: "random_local",
			Name:      fmt.Sprintf("%d_latency_seconds", size),
			Buckets:   prometheus.LinearBuckets(0, 2, 10), // 0-2 seconds
		},
		[]string{"pop"},
	)
	fetch_time := prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: "gatewaymonitor_task",
			Subsystem: "random_local",
			Name:      fmt.Sprintf("%d_fetch_seconds", size),
			Buckets:   prometheus.LinearBuckets(0, 60, 10), // 0-60 seconds
		},
		[]string{"pop"},
	)
	fails := prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "gatewaymonitor_task",
			Subsystem: "random_local",
			Name:      fmt.Sprintf("%d_fail_count", size),
		},
		[]string{"pop"},
	)
	errors := prometheus.NewCounter(
		prometheus.CounterOpts{
			Namespace: "gatewaymonitor_task",
			Subsystem: "random_local",
			Name:      fmt.Sprintf("%d_error_count", size),
		})
	reg := task.Registration{
		Schedule: schedule,
		Collectors: []prometheus.Collector{
			start_time,
			fetch_time,
			fails,
			errors,
		},
	}
	return &RandomLocalBench{
		reg:        &reg,
		size:       size,
		start_time: start_time,
		fetch_time: fetch_time,
		fails:      fails,
		errors:     errors,
	}
}

func (t *RandomLocalBench) Run(ctx context.Context, sh *shell.Shell, ps *pinning.Client, gw string) error {
	defer gc(ctx, sh)

	// generate random data
	log.Infof("generating %d bytes random data", t.size)
	randb := make([]byte, t.size)
	if _, err := rand.Read(randb); err != nil {
		t.errors.Inc()
		return fmt.Errorf("failed to generate random values: %w", err)
	}
	buf := bytes.NewReader(randb)

	// add to local ipfs
	log.Info("writing data to local IPFS node")
	cidstr, err := sh.Add(buf)
	if err != nil {
		t.errors.Inc()
		return fmt.Errorf("failed to write to IPFS: %w", err)
	}
	defer func() {
		log.Info("cleaning up IPFS node")
		err := sh.Unpin(cidstr)
		if err != nil {
			log.Warnw("failed to clean unpin cid.", "cid", cidstr)
			t.errors.Inc()
		}
	}()

	// request from gateway, observing client metrics
	url := fmt.Sprintf("%s/ipfs/%s", gw, cidstr)
	log.Infow("fetching from gateway", "url", url)
	req, _ := http.NewRequest("GET", url, nil)
	start := time.Now()
	var firstByteTime time.Time
	trace := &httptrace.ClientTrace{
		GotFirstResponseByte: func() {
			latency := time.Since(start).Seconds()
			log.Infow("first byte received", "seconds", latency)
			firstByteTime = time.Now()
		},
	}
	req = req.WithContext(httptrace.WithClientTrace(ctx, trace))
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.errors.Inc()
		return fmt.Errorf("failed to fetch from gateway %w", err)
	}
	respb, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		t.errors.Inc()
		return fmt.Errorf("failed to download content: %w", err)
	}

	labels := prometheus.Labels{
		"pop": resp.Header.Get("X-IPFS-POP"),
	}

	// Record observations.
	timeToFirstByte := firstByteTime.Sub(start).Seconds()
	totalTime := time.Since(start).Seconds()
	downloadTime := time.Since(firstByteTime).Seconds()
	downloadBytesPerSecond := float64(t.size) / downloadTime

	t.start_time.With(labels).Observe(float64(timeToFirstByte))
	common_fetch_latency.Set(float64(timeToFirstByte))

	log.Infow("finished download", "seconds", totalTime)
	t.fetch_time.With(labels).Observe(float64(totalTime))
	common_fetch_speed.Set(downloadBytesPerSecond)

	log.Info("checking result")
	// compare response with what we sent
	if !reflect.DeepEqual(respb, randb) {
		t.fails.With(labels).Inc()
		return fmt.Errorf("expected response from gateway to match generated content: %s", url)
	}

	return nil
}

func (t *RandomLocalBench) Registration() *task.Registration {
	return t.reg
}
