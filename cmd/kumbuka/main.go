package main

import (
	"context"
	"os"

	"github.com/kumbuka-me/kumbuka/internal/app"
	"github.com/kumbuka-me/kumbuka/web"
)

var (
	Version = "dev"
	Commit  = "none"
)

// main runs the Kumbuka server and exits non-zero on failure.
func main() {
	if err := app.Run(
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
