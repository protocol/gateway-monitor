package tasks

import (
	"context"
	"crypto/rand"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httptrace"
	"time"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/ipfs/go-cid"
	shell "github.com/ipfs/go-ipfs-api"
	pinning "github.com/ipfs/go-pinning-service-http-client"
	"github.com/multiformats/go-multihash"

	"github.com/ipfs-shipyard/gateway-monitor/pkg/task"
)

type NonExistCheck struct {
	reg        *task.Registration
	start_time *prometheus.HistogramVec
	fetch_time *prometheus.HistogramVec
	fails      *prometheus.CounterVec
	errors     prometheus.Counter
}

func NewNonExistCheck(schedule string) *NonExistCheck {
	start_time := prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: "gatewaymonitor_task",
			Subsystem: "non_exsist",
			Name:      "latency_seconds",
			Buckets:   prometheus.LinearBuckets(0, 60, 10), // 0-10-minutes
		},
		[]string{"pop"},
	)
	fetch_time := prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: "gatewaymonitor_task",
			Subsystem: "non_exist",
			Name:      "fetch_seconds",
			Buckets:   prometheus.LinearBuckets(0, 0.1, 10), // 0-1 second. This should never happen in reality.
		},
		[]string{"pop"},
	)
	fails := prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "gatewaymonitor_task",
			Subsystem: "non_exist",
			Name:      "fail_count",
		},
		[]string{"pop"},
	)
	errors := prometheus.NewCounter(
		prometheus.CounterOpts{
			Namespace: "gatewaymonitor_task",
			Subsystem: "non_exist",
			Name:      "error_count",
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
	return &NonExistCheck{
		reg:        &reg,
		start_time: start_time,
		fetch_time: fetch_time,
		fails:      fails,
		errors:     errors,
	}
}

func (t *NonExistCheck) Run(ctx context.Context, sh *shell.Shell, ps *pinning.Client, gw string) error {
	buf := make([]byte, 128)
	_, err := rand.Read(buf)
	if err != nil {
		t.errors.Inc()
		return fmt.Errorf("failed to generate random bytes: %w", err)
	}

	encoded, err := multihash.EncodeName(buf, "sha3")
	if err != nil {
		t.errors.Inc()
		return fmt.Errorf("failed to generate multihash of random bytes: %w", err)
	}
	cast, err := multihash.Cast(encoded)
	if err != nil {
		t.errors.Inc()
		return fmt.Errorf("failed to cast as multihash: %w", err)
	}

	c := cid.NewCidV1(cid.Raw, cast)
	log.Info("generated random CID", "cid", c.String())

	url := fmt.Sprintf("%s/ipfs/%s", gw, c.String())

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
	_, err = ioutil.ReadAll(resp.Body)
	if err != nil {
		t.errors.Inc()
		return fmt.Errorf("failed to download content: %w", err)
	}

	pop := resp.Header.Get("X-IPFS-POP")
	labels := prometheus.Labels{
		"pop": pop,
	}

	// Record observations.
	timeToFirstByte := firstByteTime.Sub(start).Seconds()
	totalTime := time.Since(start).Seconds()

	t.start_time.With(labels).Observe(float64(timeToFirstByte))

	log.Infow("finished download", "seconds", totalTime, "pop", pop)
	t.fetch_time.With(labels).Observe(float64(totalTime))

	log.Info("checking that we got a 404")
	if resp.StatusCode != 404 {
		t.fails.With(labels).Inc()
		return fmt.Errorf("expected to see 404 from gateway, but didn't. pop: %s, status: (%d): %w", pop, resp.StatusCode, err)
	}

	return nil
}

func (t *NonExistCheck) Registration() *task.Registration {
	return t.reg
}
