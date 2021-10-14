package task

import (
	"context"

	"github.com/prometheus/client_golang/prometheus"

	shell "github.com/ipfs/go-ipfs-api"
	pinning "github.com/ipfs/go-pinning-service-http-client"
)

type Task interface {
	Run(context.Context, *shell.Shell, *pinning.Client, string) error
	Registration() *Registration
}

type Registration struct {
	Collectors []prometheus.Collector
	Schedule   string
}
