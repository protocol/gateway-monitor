package tasks

import (
	"context"
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

type KnownGoodCheck struct {
	reg        *task.Registration
	checks     map[string][]byte
	start_time *prometheus.HistogramVec
	fetch_time *prometheus.HistogramVec
	fails      *prometheus.CounterVec
	errors     prometheus.Counter
}

func NewKnownGoodCheck(schedule string, checks map[string][]byte) *KnownGoodCheck {
	start_time := prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: "gatewaymonitor_task",
			Subsystem: "known_good",
			Name:      "latency",
			Buckets:   prometheus.LinearBuckets(0, 10000, 10), // 0-100 seconds
		},
		[]string{},
	)
	fetch_time := prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: "gatewaymonitor_task",
			Subsystem: "known_good",
			Name:      "fetch_time",
			Buckets:   prometheus.LinearBuckets(0, 1000, 10), // 0-1 seconds (small file)
		},
		[]string{},
	)
	fails := prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "gatewaymonitor_task",
			Subsystem: "known_good",
			Name:      "fail_count",
		},
		[]string{},
	)
	errors := prometheus.NewCounter(
		prometheus.CounterOpts{
			Namespace: "gatewaymonitor_task",
			Subsystem: "known_good",
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
	return &KnownGoodCheck{
		reg:        &reg,
		checks:     checks,
		start_time: start_time,
		fetch_time: fetch_time,
		fails:      fails,
		errors:     errors,
	}
}

func (t *KnownGoodCheck) Run(ctx context.Context, sh *shell.Shell, ps *pinning.Client, gw string) error {
	for ipfspath, value := range t.checks {
		// request from gateway, observing client metrics
		url := fmt.Sprintf("%s%s", gw, ipfspath)
		log.Infow("fetching from gateway", "url", url)
		req, _ := http.NewRequest("GET", url, nil)
		start := time.Now()
		var firstByteTime time.Time
		trace := &httptrace.ClientTrace{
			GotFirstResponseByte: func() {
				latency := time.Since(start).Milliseconds()
				log.Infow("first byte received", "ms", latency)
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
		timeToFirstByte := firstByteTime.Sub(start).Milliseconds()
		totalTime := time.Since(start).Milliseconds()

		t.start_time.With(labels).Observe(float64(timeToFirstByte))
		common_fetch_latency.Set(float64(timeToFirstByte))

		log.Infow("finished download", "ms", totalTime)
		t.fetch_time.With(labels).Observe(float64(totalTime))

		log.Info("checking result")
		// compare response with what we sent
		if !reflect.DeepEqual(respb, value) {
			t.fails.With(labels).Inc()
			return fmt.Errorf("expected response from gateway to match generated content: %s", url)
		}
	}

	return nil
}

func (t *KnownGoodCheck) Registration() *task.Registration {
	return t.reg
}
