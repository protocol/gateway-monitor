package task

import (
	"context"

	shell "github.com/ipfs/go-ipfs-api"
	pinning "github.com/ipfs/go-pinning-service-http-client"
)

type TerminalTask struct {
	Done chan bool
}

func (t *TerminalTask) Run(context.Context, *shell.Shell, *pinning.Client, string) error {
	t.Done <- true
	return nil
}

func (t *TerminalTask) Registration() *Registration {
	return new(Registration)
}
