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
	logging "github.com/ipfs/go-log"

	"github.com/ipfs-shipyard/gateway-monitor/pkg/task"
)

// This file contains the list of tasks to be run (see All)
// as well as common metrics that might be useful for more than one task.

func init() {
	prometheus.Register(fetch_speed)
	prometheus.Register(fetch_latency)
	prometheus.Register(fails)
	prometheus.Register(errors)
}

const (
	kiB = 1024
	miB = 1024 * kiB
	giB = 1024 * miB
)

var (
	log = logging.Logger("tasks")

	defaultLabels = []string{"test", "pop", "location", "size", "code"}

	All = []task.Task{
		NewRandomLocalBench("10,30,50 * * * *", 16*miB),
		NewRandomLocalBench("20 * * * *", 256*miB),
		NewIpnsBench("10,30,50 * * * *", 16*miB),
		NewIpnsBench("40 * * * *", 256*miB),
		NewKnownGoodCheck("* * * * *", map[string][]byte{
			"/ipfs/Qmc5gCcjYypU7y28oCALwfSvxCBskLuPKWpK4qpterKC7z": []byte("Hello World!\r\n"),
		}),
		NewNonExistCheck("0 * * * *"),
	}

	// Histogram metrics are defined in each test because the buckets are different between tests
	// Yes, it's annoying (especially when creating the dashboards)

	fetch_speed = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "gatewaymonitor_task",
			Subsystem: "common",
			Name:      "fetch_speed_bytes_per_second",
		},
		defaultLabels)
	fetch_latency = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "gatewaymonitor_task",
			Subsystem: "common",
			Name:      "fetch_latency_seconds",
		},
		defaultLabels)
	fails = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "gatewaymonitor_task",
			Subsystem: "common",
			Name:      "fail_count",
		},
		defaultLabels)
	errors = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "gatewaymonitor_task",
			Subsystem: "common",
			Name:      "error_count",
		},
		defaultLabels)
)

// This is here to keep the volume size down
// Tasks that create pins should clean up after themselves
// and run this.
func gc(ctx context.Context, sh *shell.Shell) error {
	log.Info("GCing repo")
	req := sh.Request("repo/gc")
	_, err := req.Send(ctx)
	if err != nil {
		log.Warnw("failed to gc repo.", "err", err)
	}
	return err
}

func addRandomData(sh *shell.Shell, t task.Task, size int) (string, []byte, error) {
	localLabels := task.Labels(t, "localhost", size, 0)

	// generate random data
	log.Infof("%s(%d): generating %d bytes random data", t.Name(), size, size)
	randb := make([]byte, size)
	if _, err := rand.Read(randb); err != nil {
		errors.With(localLabels).Inc()
		return "", []byte{}, fmt.Errorf("%s(%d): failed to generate random values: %w", t.Name(), size, err)
	}
	buf := bytes.NewReader(randb)

	// add to local ipfs
	log.Infof("%s(%d): writing data to local IPFS node", t.Name(), size)
	cidstr, err := sh.Add(buf)
	if err != nil {
		errors.With(localLabels).Inc()
		return "", []byte{}, fmt.Errorf("%s(%d): failed to write to IPFS: %w", t.Name(), size, err)
	}

	return cidstr, randb, nil
}

func checkAndRecord(
	ctx context.Context,
	t task.Task,
	gw string,
	url string,
	expected []byte,
) error {
	size := len(expected)
	remoteLabels := task.Labels(t, gw, size, 0)

	log.Infof("%s(%d): fetching from gateway. url: %s", t.Name(), size, url)
	req, _ := http.NewRequest("GET", url, nil)
	start := time.Now()

	var firstByteTime time.Time

	trace := &httptrace.ClientTrace{
		GotFirstResponseByte: func() {
			latency := time.Since(start).Seconds()
			log.Infof("%s(%d): first byte received in %f seconds", t.Name(), size, latency)
			firstByteTime = time.Now()
		},
	}

	req = req.WithContext(httptrace.WithClientTrace(ctx, trace))
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		errors.With(remoteLabels).Inc()
		return fmt.Errorf("%s(%d): failed to fetch from gateway %w", t.Name(), size, err)
	}

	respb, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		errors.With(remoteLabels).Inc()
		return fmt.Errorf("%s(%d): failed to download content: %w", t.Name(), size, err)
	}

	pop := resp.Header.Get("X-IPFS-POP")
	if pop == "" {
		pop = resp.Header.Get("X-IPFS-LB-POP") // If go-ipfs didn't reply, get the pop from the LB
	}

	responseLabels := task.Labels(t, pop, size, resp.StatusCode)

	timeToFirstByte := firstByteTime.Sub(start).Seconds()
	totalTime := time.Since(start).Seconds()
	downloadTime := time.Since(firstByteTime).Seconds()
	downloadBytesPerSecond := float64(size) / downloadTime

	fetch_latency.With(responseLabels).Set(float64(timeToFirstByte))
	var labMap map[string]string = *(&responseLabels)

	// Record results
	if t.LatencyHist() != nil {
		log.Infof("Publishing latency histogram %s with labels %v", t.Name(), labMap)
		t.LatencyHist().With(responseLabels).Observe(float64(timeToFirstByte))
	}
	if t.FetchHist() != nil {
		log.Infof("Publishing fetch histogram %s with labels %v", t.Name(), labMap)
		t.FetchHist().With(responseLabels).Observe(float64(totalTime))
	}

	if resp.StatusCode != 200 {
		fails.With(responseLabels).Inc()

		downloadBytesPerSecond = float64(resp.ContentLength) / downloadTime
		fetch_speed.With(responseLabels).Set(downloadBytesPerSecond)

		return fmt.Errorf("%s(%d): expected response code 200 from gateway, got %d from %s. url: %s", t.Name(), size, resp.StatusCode, pop, url)
	}

	fetch_speed.With(responseLabels).Set(downloadBytesPerSecond)
	log.Infof("%s(%d): finished download in %f seconds. speed: %f bytes/sec. pop: %s", t.Name(), totalTime, size, downloadBytesPerSecond, pop)

	// compare response with what we sent
	log.Infof("%s(%d): checking result", t.Name(), size)
	if !reflect.DeepEqual(expected, respb) {
		fails.With(responseLabels).Inc()
		return fmt.Errorf("%s(%d): expected response from gateway to match generated content. pop: %s, url: %s", t.Name(), size, pop, resp.Request.URL)
	}
	return nil
}
