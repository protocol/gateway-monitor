package tasks

import (
	"context"
	"crypto/rand"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httptrace"
	"strconv"
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
	latency    *prometheus.HistogramVec
	fetch_time *prometheus.HistogramVec
	errors     *prometheus.CounterVec
}

func NewNonExistCheck(schedule string) *NonExistCheck {
	start_time := prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: "gatewaymonitor_task",
			Subsystem: "non_exist",
			Name:      "latency_seconds",
			Buckets:   prometheus.LinearBuckets(0, 30, 20), // 0-10-minutes
		},
		[]string{"pop"},
	)
	fetch_time := prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: "gatewaymonitor_task",
			Subsystem: "non_exist",
			Name:      "fetch_seconds",
			Buckets:   prometheus.LinearBuckets(0, 0.2, 10), // 0-1 second. This should never happen in reality.
		},
		[]string{"pop"},
	)
	// We track errors separately since for this metric an error is actually a success
	errors := prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "gatewaymonitor_task",
			Subsystem: "non_exist",
			Name:      "error_count",
		},
		[]string{"pop"},
	)
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
		latency:    start_time,
		fetch_time: fetch_time,
		errors:     errors,
	}
}

func (t *NonExistCheck) Name() string {
	return "non_exist"
}

func (t *NonExistCheck) Run(ctx context.Context, sh *shell.Shell, ps *pinning.Client, gw string) error {
	localLabels := prometheus.Labels{"pop": "localhost"}
	remoteLabels := prometheus.Labels{"pop": gw}

	buf := make([]byte, 128)
	_, err := rand.Read(buf)
	if err != nil {
		t.errors.With(localLabels).Inc()
		return fmt.Errorf("failed to generate random bytes: %w", err)
	}

	encoded, err := multihash.EncodeName(buf, "sha3")
	if err != nil {
		t.errors.With(localLabels).Inc()
		return fmt.Errorf("failed to generate multihash of random bytes: %w", err)
	}
	cast, err := multihash.Cast(encoded)
	if err != nil {
		t.errors.With(localLabels).Inc()
		return fmt.Errorf("failed to cast as multihash: %w", err)
	}

	c := cid.NewCidV1(cid.Raw, cast)
	log.Infof("generated random CID %s", c)

	url := fmt.Sprintf("%s/ipfs/%s", gw, c)

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
		t.errors.With(remoteLabels).Inc()
		return fmt.Errorf("failed to fetch from gateway: %w", err)
	}
	_, err = ioutil.ReadAll(resp.Body)
	if err != nil {
		t.errors.With(remoteLabels).Inc()
		return fmt.Errorf("failed to download content: %w", err)
	}

	pop := resp.Header.Get("X-IPFS-POP")
	if pop == "" {
		pop = resp.Header.Get("X-IPFS-LB-POP") // If go-ipfs didn't reply, get the pop from the LB
	}

	log.Info("checking that we got a 404 or 504")
	if resp.StatusCode != 404 && resp.StatusCode != 504 {
		errorLabels := prometheus.Labels{
			"test": "nonexist",
			"size": "",
			"pop":  pop,
			"code": strconv.Itoa(resp.StatusCode),
		}
		fails.With(errorLabels).Inc()
		return fmt.Errorf("expected to see 404 or 504 from gateway, but didn't. pop: %s, status: (%d)", pop, resp.StatusCode)
	}

	responseLabels := prometheus.Labels{"test": "nonexist", "size": "", "pop": pop, "code": strconv.Itoa(resp.StatusCode)}
	popLabels := prometheus.Labels{"pop": pop}

	// Record observations.
	timeToFirstByte := firstByteTime.Sub(start).Seconds()
	totalTime := time.Since(start).Seconds()

	t.latency.With(popLabels).Observe(float64(timeToFirstByte))
	fetch_latency.With(responseLabels).Set(float64(timeToFirstByte))

	log.Infow("finished download", "seconds", totalTime, "pop", pop)
	t.fetch_time.With(popLabels).Observe(float64(totalTime))

	return nil
}

func (t *NonExistCheck) Registration() *task.Registration {
	return t.reg
}
