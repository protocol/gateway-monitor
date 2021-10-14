package commands

import (
	"github.com/urfave/cli/v2"

	shell "github.com/ipfs/go-ipfs-api"
	logging "github.com/ipfs/go-log"
	pinning "github.com/ipfs/go-pinning-service-http-client"
)

var (
	log = logging.Logger("gatewaymonitor")
	All = []*cli.Command{
		singleCommand,
		daemonCommand,
	}
)

// utility functions
func GetIPFS(cctx *cli.Context) *shell.Shell {
	sh := new(shell.Shell)
	if cctx.IsSet("ipfs") {
		sh = shell.NewShell(cctx.String("ipfs"))
	} else {
		sh = shell.NewLocalShell()
	}

	return sh
}

func GetGW(cctx *cli.Context) string {
	args := cctx.Args()
	if len(args.Slice()) > 0 {
		return args.First()
	}
	return "https://ipfs.io"
}

func GetPinningService(cctx *cli.Context) *pinning.Client {
	if cctx.IsSet("pinning-service") && cctx.IsSet("pinning-token") {
		url := cctx.String("pinning-service")
		tok := cctx.String("pinning-token")
		return pinning.NewClient(url, tok)
	}
	return nil
}
