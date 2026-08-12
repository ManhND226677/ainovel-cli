package webapi

import (
	"github.com/voocel/ainovel-cli/assets"
	"github.com/voocel/ainovel-cli/internal/bootstrap"
	"github.com/voocel/ainovel-cli/internal/host"
)

// Run starts the local API process. The browser controls the same Host
// instance, so snapshots and replayed events are from the live engine.
func Run(cfg bootstrap.Config, bundle assets.Bundle, addr string) error {
	runtime, err := host.New(cfg, bundle, host.WithFileLog("web.log", false))
	if err != nil {
		return err
	}
	defer runtime.Close()
	return New(runtime).Serve(Options{Addr: addr})
}
