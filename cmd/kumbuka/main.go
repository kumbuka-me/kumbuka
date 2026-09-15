package main

import (
	"context"
	"os"

	"github.com/kumbuka-me/kumbuka/internal/cli"
	"github.com/kumbuka-me/kumbuka/web"
)

var (
	Version = "dev"
	Commit  = "none"
)

// main runs the Kumbuka command-line application and exits non-zero on failure.
func main() {
	if err := cli.Run(
		context.Background(),
		os.Args[1:],
		web.Assets,
		Version,
		Commit,
		os.Stdout,
		os.Stderr,
	); err != nil {
		os.Exit(1)
	}
}
