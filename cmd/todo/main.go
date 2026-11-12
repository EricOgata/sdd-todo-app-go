// Command todo is a single-user, local-first task manager for the terminal.
package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"todo/internal/cli"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	app := cli.NewApp(cli.Options{Out: os.Stdout, Err: os.Stderr})
	os.Exit(app.Execute(ctx))
}
