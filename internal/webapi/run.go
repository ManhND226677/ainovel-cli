package webapi

import (
	"github.com/voocel/ainovel-cli/assets"
	"github.com/voocel/ainovel-cli/internal/bootstrap"
	"github.com/voocel/ainovel-cli/internal/host"
)

// Run starts the local API process. The browser controls the same Host
// instance, so snapshots and replayed events are from the live engine.
//
// opts.Addr defaults to 127.0.0.1:10001 when empty.
// opts.Token protects mutating control routes when non-empty.
// opts.UIDir, when set, serves a built web dashboard (SPA) from that directory
// on the same listener as /api/*.
func Run(cfg bootstrap.Config, bundle assets.Bundle, opts Options) error {
	runtime, err := host.New(cfg, bundle, host.WithFileLog("web.log", false))
	if err != nil {
		return err
	}
	defer runtime.Close()
	return New(runtime).Serve(opts)
}
