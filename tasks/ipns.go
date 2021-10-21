package tasks

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
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

type IpnsBench struct {
	reg          *task.Registration
	size         int
	publish_time prometheus.Histogram
	start_time   *prometheus.HistogramVec
	fetch_time   *prometheus.HistogramVec
	fails        *prometheus.CounterVec
	errors       prometheus.Counter
}

func NewIpnsBench(schedule string, size int) *IpnsBench {
	publish_time := prometheus.NewHistogram(
		prometheus.HistogramOpts{
			Namespace: "gatewaymonitor_task",
			Subsystem: "ipns",
			Name:      "publish_seconds",
			Buckets:   prometheus.LinearBuckets(0, 10, 10), // 0-10 seconds
		},
	)
	start_time := prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: "gatewaymonitor_task",
			Subsystem: "ipns",
			Name:      fmt.Sprintf("%d_latency_seconds", size),
			Buckets:   prometheus.LinearBuckets(0, 6, 10), // 0-1 minutes
		},
		[]string{"pop"},
	)
	fetch_time := prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: "gatewaymonitor_task",
			Subsystem: "ipns",
			Name:      fmt.Sprintf("%d_fetch_seconds", size),
			Buckets:   prometheus.LinearBuckets(0, 6, 15), // 0-1:30 minutes
		},
		[]string{"pop"},
	)
	fails := prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "gatewaymonitor_task",
			Subsystem: "ipns",
			Name:      fmt.Sprintf("%d_fail_count", size),
		},
		[]string{"pop"},
	)
	errors := prometheus.NewCounter(
		prometheus.CounterOpts{
			Namespace: "gatewaymonitor_task",
			Subsystem: "ipns",
			Name:      fmt.Sprintf("%d_error_count", size),
		},
	)
	reg := task.Registration{
		Schedule: schedule,
		Collectors: []prometheus.Collector{
			publish_time,
			start_time,
			fetch_time,
			fails,
			errors,
		},
	}
	return &IpnsBench{
		reg:          &reg,
		size:         size,
		publish_time: publish_time,
		start_time:   start_time,
		fetch_time:   fetch_time,
		fails:        fails,
		errors:       errors,
	}
}

func (t *IpnsBench) Run(ctx context.Context, sh *shell.Shell, ps *pinning.Client, gw string) error {
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
		log.Errorw("failed to write to IPFS", "err", err)
		t.errors.Inc()
		return err
	}
	defer func() {
		log.Info("cleaning up IPFS node")
		err := sh.Unpin(cidstr)
		if err != nil {
			t.errors.Inc()
			log.Warnw("failed to clean unpin cid.", "cid", cidstr)
		}
	}()

	// Generate a new key
	// we already have a random value lying around, might as
	// well use it for the ney name.
	keyName := base64.StdEncoding.EncodeToString(randb[:8])
	_, err = sh.KeyGen(ctx, keyName)
	if err != nil {
		t.errors.Inc()
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
		return fmt.Errorf("failed to fetch from gateway: %w", err)
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
		return fmt.Errorf("expected response from gateway to match generated content: %w", err)
	}

	return nil
}

func (t *IpnsBench) Registration() *task.Registration {
	return t.reg
}
