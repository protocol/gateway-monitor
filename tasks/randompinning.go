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

	"github.com/ipfs/go-cid"
	shell "github.com/ipfs/go-ipfs-api"
	pinning "github.com/ipfs/go-pinning-service-http-client"

	"github.com/ipfs-shipyard/gateway-monitor/pkg/task"
)

type RandomPinningBench struct {
	reg        *task.Registration
	size       int
	start_time prometheus.Histogram
	fetch_time prometheus.Histogram
	fails      prometheus.Counter
	errors     prometheus.Counter
}

func NewRandomPinningBench(schedule string, size int) *RandomPinningBench {
	start_time := prometheus.NewHistogram(
		prometheus.HistogramOpts{
			Namespace: "gatewaymonitor_task",
			Subsystem: "random_pinning",
			Name:      fmt.Sprintf("%d_latency", size),
			Buckets:   prometheus.LinearBuckets(0, 10000, 10), // 0-10 seconds
		})
	fetch_time := prometheus.NewHistogram(
		prometheus.HistogramOpts{
			Namespace: "gatewaymonitor_task",
			Subsystem: "random_pinning",
			Name:      fmt.Sprintf("%d_fetch_time", size),
			Buckets:   prometheus.LinearBuckets(0, 300000, 10), // 0-300 seconds
		})
	fails := prometheus.NewCounter(
		prometheus.CounterOpts{
			Namespace: "gatewaymonitor_task",
			Subsystem: "random_pinning",
			Name:      fmt.Sprintf("%d_fail_count", size),
		})
	errors := prometheus.NewCounter(
		prometheus.CounterOpts{
			Namespace: "gatewaymonitor_task",
			Subsystem: "random_pinning",
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
	return &RandomPinningBench{
		reg:        &reg,
		size:       size,
		start_time: start_time,
		fetch_time: fetch_time,
		fails:      fails,
		errors:     errors,
	}
}

func (t *RandomPinningBench) Run(ctx context.Context, sh *shell.Shell, ps *pinning.Client, gw string) error {
	defer cleanup(ctx, sh)

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
		log.Errorw("failed to write to IPFS: %w", err)
	}
	defer func() {
		log.Info("cleaning up IPFS node")
		// don't bother error checking. We clean it up explicitly in the happy path.
		sh.Unpin(cidstr)
	}()

	// Pin to pinning service
	c, err := cid.Decode(cidstr)
	if err != nil {
		t.errors.Inc()
		return fmt.Errorf("failed to decode cid after it was returned from IPFS: %w", err)
	}
	getter, err := ps.Add(ctx, c)
	if err != nil {
		t.errors.Inc()
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
			fmt.Println(err)
		}
		time.Sleep(time.Minute)
	}

	// delete this from our local IPFS node.
	log.Info("removing pin from local IPFS node")
	err = sh.Unpin(cidstr)
	if err != nil {
		t.errors.Inc()
		return fmt.Errorf("could not unpin cid after adding it earlier: %w", err)
	}

	// request from gateway, observing client metrics
	url := fmt.Sprintf("%s/ipfs/%s", gw, cidstr)
	log.Infow("fetching from gateway", "url", url)
	req, _ := http.NewRequest("GET", url, nil)
	start := time.Now()
	var firstbyte_time time.Time
	trace := &httptrace.ClientTrace{
		GotFirstResponseByte: func() {
			latency := time.Since(start).Milliseconds()
			log.Infow("first byte received", "ms", latency)
			t.start_time.Observe(float64(latency))
			common_fetch_latency.Set(float64(latency))
			firstbyte_time = time.Now()
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
		return fmt.Errorf("failed to downlaod content: %w", err)
	}
	total_time := time.Since(start).Milliseconds()
	download_time := time.Since(firstbyte_time).Seconds()
	log.Infow("finished download", "ms", total_time)
	t.fetch_time.Observe(float64(total_time))
	downloadBytesPerSecond := float64(t.size) / download_time
	common_fetch_speed.Set(downloadBytesPerSecond)

	log.Info("checking result")
	// compare response with what we sent
	if !reflect.DeepEqual(respb, randb) {
		t.fails.Inc()
		return fmt.Errorf("expected response from gateway to match generated content: %s", url)
	}

	return nil
}

func (t *RandomPinningBench) Registration() *task.Registration {
	return t.reg
}
