package commands

import (
	"net/http"
	"os"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/urfave/cli/v2"

	logging "github.com/ipfs/go-log"

	"github.com/ipfs-shipyard/gateway-monitor/pkg/engine"
	"github.com/ipfs-shipyard/gateway-monitor/tasks"
)

var singleCommand = &cli.Command{
	Name:    "single",
	Aliases: []string{},
	Usage:   "run tests once, ignoring the schedule",
	Action: func(cctx *cli.Context) error {
		log = logging.Logger("main")
		if _, found := os.LookupEnv("GOLOG_LOG_LEVEL"); !found {
			logging.SetAllLoggers(logging.LevelInfo)
		}

		ipfs := GetIPFS(cctx)
		ps := GetPinningService(cctx)
		gw := GetGW(cctx)

		for _, t := range tasks.All {
			for _, col := range t.Registration().Collectors {
				prometheus.Register(col)
			}
		}

		http.Handle("/metrics", promhttp.Handler())

		go func() {
			http.ListenAndServe(":2112", nil)
		}()

		log.Info("Prometheus metrics listener running at http://0.0.0.0:2112/metrics")

		eng := engine.NewSingle(ipfs, ps, gw)

		if cctx.IsSet("loop") {
			eng.AddTask(eng.RepeatForever(tasks.All))
			log.Info("Looping forever")
			engCh := eng.Start(cctx.Context)

			for {
				err := <-engCh
				log.Error(err)
			}
		} else {
			for _, t := range tasks.All {
				eng.AddTask(t)
			}
			eng.AddTask(eng.TerminalTask())
			return <-eng.Start(cctx.Context)
		}
	},
	Flags: []cli.Flag{
		&cli.BoolFlag{
			Name:  "loop",
			Usage: "loop forever, running each test one after another, ignoring the schedule",
		},
	},
}
