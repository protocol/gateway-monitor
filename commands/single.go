package commands

import (
	"os"

	"github.com/urfave/cli/v2"

	logging "github.com/ipfs/go-log"

	"github.com/ipfs-shipyard/gateway-monitor/pkg/engine"
	"github.com/ipfs-shipyard/gateway-monitor/tasks"
)

var singleCommand = &cli.Command{
	Name:  "single",
	Usage: "run tests once, ignoring the schedule",
	Action: func(cctx *cli.Context) error {
		// If we arent explicitly setting the log level,
		// lets set it so most messages can be seen
		if _, found := os.LookupEnv("GOLOG_LOG_LEVEL"); !found {
			logging.SetAllLoggers(logging.LevelInfo)
		}
		ipfs := GetIPFS(cctx)
		ps := GetPinningService(cctx)
		gw := GetGW(cctx)
		eng := engine.NewSingle(ipfs, ps, gw, tasks.All...)
		return <-eng.Start(cctx.Context)
	},
}
