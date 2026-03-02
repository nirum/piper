package main

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/niru/piper/cmd"
)

// Set by goreleaser at build time.
var version = "dev"

func main() {
	ctx, cancel := signal.NotifyContext(
		cmd.WithInterrupted(os.Stdin),
		syscall.SIGINT, syscall.SIGTERM,
	)
	defer cancel()

	code := cmd.Run(ctx, os.Args[1:], os.Stdin, os.Stdout, os.Stderr, version)
	if code != 0 {
		if cmd.WasInterrupted(ctx) {
			fmt.Fprintln(os.Stderr, "") // newline after ^C
		}
		os.Exit(code)
	}
}
