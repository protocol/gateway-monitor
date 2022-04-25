package task

import (
	"context"
	"regexp"
	"strconv"

	"github.com/prometheus/client_golang/prometheus"

	shell "github.com/ipfs/go-ipfs-api"
	pinning "github.com/ipfs/go-pinning-service-http-client"
)

type Task interface {
	Name() string
	Run(context.Context, *shell.Shell, *pinning.Client, string) error
	Registration() *Registration
	LatencyHist() *prometheus.HistogramVec
	FetchHist() *prometheus.HistogramVec
}

var popRegex = regexp.MustCompile("^[a-z0-9-]+-([a-z0-9]+)$")

func popToLocation(pop string) string {
	if popRegex.MatchString(pop) {
		return popRegex.FindStringSubmatch(pop)[1]
	}

	return pop
}

func Labels(t Task, pop string, size int, code int) prometheus.Labels {
	return prometheus.Labels{
		"test":     t.Name(),
		"pop":      pop,
		"size":     strconv.Itoa(size),
		"code":     strconv.Itoa(code),
		"location": popToLocation(pop),
	}
}

type Registration struct {
	Collectors []prometheus.Collector
	Schedule   string
}
