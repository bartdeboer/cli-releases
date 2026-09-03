package main

import (
	"context"
	"fmt"
	"github.com/bartdeboer/cli-releases/internal/cliget"
	"os"
	"os/signal"
	"syscall"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := cliget.Run(ctx, os.Args[1:], os.Stdout, os.Stderr); err != nil {
		if cliget.JSONRequested(os.Args[1:]) {
			_ = cliget.WriteError(os.Stdout, err)
		} else {
			fmt.Fprintln(os.Stderr, "error:", err)
		}
		os.Exit(1)
	}
}
