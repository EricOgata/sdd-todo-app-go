package cli

import (
	"context"

	"github.com/spf13/cobra"

	"todo/internal/apperr"
	"todo/internal/clock"
	"todo/internal/config"
	"todo/internal/db"
	"todo/internal/output"
)

// Runtime carries the per-command services that later features consume. It is
// attached to the command context by the root persistent pre-run, so commands
// never resolve paths or open the database themselves.
type Runtime struct {
	Paths   config.Paths
	Store   *db.Store
	Printer *output.Printer
	Clock   clock.Clock
}

type runtimeKey struct{}

func withRuntime(ctx context.Context, rt *Runtime) context.Context {
	return context.WithValue(ctx, runtimeKey{}, rt)
}

// RuntimeFrom returns the runtime bound to cmd. It is available in every
// command that needs storage.
func RuntimeFrom(cmd *cobra.Command) (*Runtime, error) {
	rt, ok := cmd.Context().Value(runtimeKey{}).(*Runtime)
	if !ok || rt == nil {
		return nil, apperr.System("internal error: command runtime is not initialized")
	}
	return rt, nil
}

// MustRuntimeFrom returns the runtime or panics. It is reserved for commands
// whose pre-run guarantees initialization and is never used on a user path.
func MustRuntimeFrom(cmd *cobra.Command) *Runtime {
	rt, err := RuntimeFrom(cmd)
	if err != nil {
		panic(err)
	}
	return rt
}
