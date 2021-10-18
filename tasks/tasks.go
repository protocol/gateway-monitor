package tasks

import (
	"context"

	"github.com/prometheus/client_golang/prometheus"

	shell "github.com/ipfs/go-ipfs-api"
	logging "github.com/ipfs/go-log"

	"github.com/ipfs-shipyard/gateway-monitor/pkg/task"
)

// This file contains the list of tasks to be run (see All)
// as well as common metrics that might be useful for more than one task.

func init() {
	prometheus.Register(common_fetch_speed)
	prometheus.Register(common_fetch_latency)
}

const (
	kiB = 1024
	miB = 1024 * kiB
	giB = 1024 * miB
)

var (
	log = logging.Logger("tasks")

	All = []task.Task{
		NewRandomLocalBench("0 * * * *", 16*miB),
		NewRandomLocalBench("0 * * * *", 256*miB),
		NewIpnsBench("0 * * * *", 16*miB),
		NewIpnsBench("0 * * * *", 256*miB),
		NewKnownGoodCheck("0 * * * *", map[string][]byte{
			"/ipfs/Qmc5gCcjYypU7y28oCALwfSvxCBskLuPKWpK4qpterKC7z": []byte("Hello World!\r\n"),
		}),
		// NewNonExistCheck("0 * * * *"),
	}

	common_fetch_speed = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Namespace: "gatewaymonitor_task",
			Subsystem: "common",
			Name:      "fetch_speed",
		})
	common_fetch_latency = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Namespace: "gatewaymonitor_task",
			Subsystem: "common",
			Name:      "fetch_latency",
		})
)

// Clean up the
func cleanup(ctx context.Context, sh *shell.Shell) error {
	log.Info("cleanin up ipfs")
	infos, err := sh.Pins()
	if err != nil {
		return err
	}

	for k := range infos {
		err := sh.Unpin(k)
		if err != nil {
			log.Warn("failed to unpin from ipfs", "cid", k, "err", err)
			// continue. we still want to try to GC
		}
	}
	req := sh.Request("repo/gc")
	_, err = req.Send(ctx)
	if err != nil {
		log.Warn("failed to gc repo.", "err", err)
	}
	return err
}
