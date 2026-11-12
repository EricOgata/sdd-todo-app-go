package cli

import (
	"context"
	"errors"
	"fmt"
	"os"

	"todo/internal/apperr"
)

// Execute runs the command tree and returns the documented process exit code.
// It never calls os.Exit, so deferred cleanup runs before main exits.
func (a *App) Execute(ctx context.Context) int {
	a.state.store = nil
	a.state.runtime = nil
	a.state.printer = nil

	err := a.root.ExecuteContext(ctx)

	if a.state.store != nil {
		_ = a.state.store.Close()
		a.state.store = nil
	}

	if err == nil || errors.Is(err, errStop) {
		return 0
	}

	err = apperr.Unexpected(apperr.ClassifyCobra(err))
	a.writeError(apperr.Message(err))
	return apperr.ExitCode(err)
}

func (a *App) writeError(msg string) {
	w := a.opts.Err
	if w == nil {
		w = os.Stderr
	}
	fmt.Fprintf(w, "error: %s\n", msg)
}
