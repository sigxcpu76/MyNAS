package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/sigxcpu/nas/internal/cli"
)

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM, syscall.SIGQUIT)
	defer cancel()

	rootCmd := cli.NewRootCmd()
	if err := rootCmd.ExecuteContext(ctx); err != nil {
		os.Exit(1)
	}
}
