package main

import (
	"log"
	"os"

	"github.com/urfave/cli/v2"

	"github.com/ipfs-shipyard/gateway-monitor/commands"
)

func main() {
	app := &cli.App{
		Name:     "gateway-monitor",
		Usage:    "monitor IPFS gateway performance",
		Commands: commands.All,
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:  "ipfs",
				Usage: "IPFS api Multiaddr (will use IPFS_PATH discovery if unset)",
				EnvVars: []string{
					"GATEWAY_MONITOR_IPFS",
				},
			},
			&cli.StringFlag{
				Name: "pinning-service",
				Aliases: []string{
					"ps",
				},
				Usage: "pinning service url",
				EnvVars: []string{
					"GATEWAY_MONITOR_PINNING_SERVICE_URL",
				},
			},
			&cli.StringFlag{
				Name: "pinning-token",
				Aliases: []string{
					"pt",
				},
				Usage: "token for pinning service",
				EnvVars: []string{
					"GATEWAY_MONITOR_PINNING_SERVICE_TOKEN",
				},
			},
		},
		EnableBashCompletion: true,
	}
	if err := app.Run(os.Args); err != nil {
		log.Fatal(err)
	}
}
