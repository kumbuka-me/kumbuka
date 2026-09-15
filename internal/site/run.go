package site

import (
	"context"
	"fmt"
	"io"
	"io/fs"

	"github.com/kumbuka-me/kumbuka/internal/logging"
)

// Run builds one filesystem-backed static documentation site.
func Run(ctx context.Context, appFS fs.FS, config Config, overrides map[string]any, stdout io.Writer) error {
	return run(ctx, newBuilder(appFS), config, overrides, stdout)
}

// run executes one site build with an already constructed builder. Production
// callers use the default builder; tests can inject a focused renderer.
func run(ctx context.Context, builder *builder, config Config, overrides map[string]any, stdout io.Writer) error {
	logger := logging.Setup(config.logFormat, false, stdout)
	setupLogger := logger.With("component", "setup")

	if len(overrides) > 0 {
		setupLogger.Info(
			"CLI Overrides",
			"event", "cli_overrides",
			"overrides", overrides,
		)
	}

	result, err := builder.build(ctx, config)
	if err != nil {
		setupLogger.Error("Build failed", "error", err)
		return err
	}

	_, _ = fmt.Fprintf(stdout, "Built %d pages into %s\n", result.pages, result.outputDir)
	return nil
}
