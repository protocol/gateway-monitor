package commands

import (
	"net/http"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/urfave/cli/v2"

	"github.com/ipfs-shipyard/gateway-monitor/pkg/engine"
	"github.com/ipfs-shipyard/gateway-monitor/tasks"
)

var errCounter = prometheus.NewCounter(
	prometheus.CounterOpts{
		Namespace: "gatewaymonitor",
		Subsystem: "daemon",
		Name:      "errors_count",
	})

func init() {
	prometheus.Register(errCounter)
}

var daemonCommand = &cli.Command{
	Name:  "daemon",
	Usage: "run commands on schedule",
	Action: func(cctx *cli.Context) error {
		ipfs := GetIPFS(cctx)
		ps := GetPinningService(cctx)
		gw := GetGW(cctx)
		eng := engine.New(ipfs, ps, gw, tasks.All...)
		go func() {
			errCh := eng.Start(cctx.Context)
			for {
				select {
				case _ = <-errCh:
					errCounter.Inc()
				}
			}
		}()
		http.Handle("/metrics", promhttp.Handler())
		return http.ListenAndServe(":2112", nil)
	},
}
