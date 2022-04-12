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
	"strconv"
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
	// Yes, it's annoying (both in code and when creating the dashboards)

	fetch_speed = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "gatewaymonitor_task",
			Name:      "fetch_speed_bytes_per_second",
		},
		[]string{"test", "pop", "size"})
	fetch_latency = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "gatewaymonitor_task",
			Name:      "fetch_latency_seconds",
		},
		[]string{"test", "pop", "size"})
	fails = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "gatewaymonitor_task",
			Name:      "fail_count",
		},
		[]string{"test", "pop", "size", "code"},
	)
	errors = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "gatewaymonitor_task",
			Name:      "error_count",
		},
		[]string{"test", "pop", "size"},
	)
)

// This is here to keep the volume size down
// Tasks that create pins should clean up after themselves
// and run this.
func gc(ctx context.Context, sh *shell.Shell) error {
	req := sh.Request("repo/gc")
	_, err := req.Send(ctx)
	if err != nil {
		log.Warnw("failed to gc repo.", "err", err)
	}
	return err
}

func addRandomData(sh *shell.Shell, taskName string, size int) (string, []byte, error) {
	localLabels := prometheus.Labels{"test": taskName, "size": strconv.Itoa(size), "pop": "localhost"}

	// generate random data
	log.Infof("%s: generating %d bytes random data", taskName, size)
	randb := make([]byte, size)
	if _, err := rand.Read(randb); err != nil {
		errors.With(localLabels).Inc()
		return "", []byte{}, fmt.Errorf("%s: failed to generate random values: %w", taskName, err)
	}
	buf := bytes.NewReader(randb)

	// add to local ipfs
	log.Infof("%s: writing data to local IPFS node", taskName)
	cidstr, err := sh.Add(buf)
	if err != nil {
		errors.With(localLabels).Inc()
		return "", []byte{}, fmt.Errorf("%s: failed to write to IPFS: %w", taskName, err)
	}

	return cidstr, randb, nil
}

func checkAndRecord(
	ctx context.Context,
	taskName string,
	gw string,
	url string,
	expected []byte,
	testLatencyHist *prometheus.HistogramVec,
	testTimeHist *prometheus.HistogramVec,
) error {
	size := len(expected)
	remoteLabels := prometheus.Labels{"test": taskName, "size": strconv.Itoa(size), "pop": gw}
	log.Infof("%s: fetching from gateway. url: %s", taskName, url)
	req, _ := http.NewRequest("GET", url, nil)
	start := time.Now()

	var firstByteTime time.Time

	trace := &httptrace.ClientTrace{
		GotFirstResponseByte: func() {
			latency := time.Since(start).Seconds()
			log.Infof("%s: first byte received in %f seconds", taskName, latency)
			firstByteTime = time.Now()
		},
	}

	req = req.WithContext(httptrace.WithClientTrace(ctx, trace))
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		errors.With(remoteLabels).Inc()
		return fmt.Errorf("%s: failed to fetch from gateway %w", taskName, err)
	}

	respb, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		errors.With(remoteLabels).Inc()
		return fmt.Errorf("%s: failed to download content: %w", taskName, err)
	}

	pop := resp.Header.Get("X-IPFS-POP")
	if pop == "" {
		pop = resp.Header.Get("X-IPFS-LB-POP") // If go-ipfs didn't reply, get the pop from the LB
	}

	if resp.StatusCode != 200 {
		errorLabels := prometheus.Labels{
			"test": taskName,
			"size": strconv.Itoa(size),
			"pop":  pop,
			"code": strconv.Itoa(resp.StatusCode),
		}
		fails.With(errorLabels).Inc()
		log.Errorf("%s: expected response code 200 from gateway, got %d from %s. url: %s", taskName, resp.StatusCode, pop, url)
		return fmt.Errorf("%s: expected response code 200 from gateway, got %d from %s. url: %s", taskName, resp.StatusCode, pop, url)
	}

	responseLabels := prometheus.Labels{"test": taskName, "size": strconv.Itoa(size), "pop": pop}
	popLabels := prometheus.Labels{"pop": pop}

	timeToFirstByte := firstByteTime.Sub(start).Seconds()
	totalTime := time.Since(start).Seconds()
	downloadTime := time.Since(firstByteTime).Seconds()
	downloadBytesPerSecond := float64(size) / downloadTime

	testLatencyHist.With(popLabels).Observe(float64(timeToFirstByte))
	fetch_latency.With(responseLabels).Set(float64(timeToFirstByte))

	log.Infof("%s: finished download in %d seconds. speed: %f bytes/sec. pop: %s", taskName, totalTime, downloadBytesPerSecond, pop)
	testTimeHist.With(popLabels).Observe(float64(totalTime))
	fetch_speed.With(responseLabels).Set(downloadBytesPerSecond)

	// compare response with what we sent
	log.Infof("%s: checking result", taskName)
	if !reflect.DeepEqual(expected, respb) {
		fails.With(responseLabels).Inc()
		log.Errorf("%s: expected response from gateway to match generated content. pop: %s, url: %s", taskName, pop, resp.Request.URL)
		return fmt.Errorf("%s: expected response from gateway to match generated content. pop: %s, url: %s", taskName, pop, resp.Request.URL)
	}
	return nil
}
