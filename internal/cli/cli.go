// Package cli defines Kumbuka's command tree and binds command-line flags to application operations.
package cli

import (
	"context"
	"fmt"
	"io"
	"io/fs"

	"github.com/containeroo/tinyflags"
	"github.com/kumbuka-me/kumbuka/internal/mirror"
	"github.com/kumbuka-me/kumbuka/internal/pluginproject"
	"github.com/kumbuka-me/kumbuka/internal/serve"
	"github.com/kumbuka-me/kumbuka/internal/site"
	"github.com/kumbuka-me/kumbuka/plugins"
)

// Run parses and executes one Kumbuka command.
func Run(
	ctx context.Context,
	args []string,
	appFS fs.FS,
	version, commit string,
	stdout, stderr io.Writer,
) error {
	root := tinyflags.NewCommand("kumbuka", tinyflags.ContinueOnError).RequireCommand()

	root.Version(version)
	root.EnvPrefix("KUMBUKA_")

	serveCommand := root.Command("serve", "Run the Kumbuka server")
	resolveServeConfig := serve.BindFlags(serveCommand.FlagSet)

	serveCommand.Run(func(ctx context.Context) error {
		return serve.Run(
			ctx,
			appFS,
			resolveServeConfig(),
			serveCommand.OverriddenValues(),
			version,
			commit,
			stdout,
		)
	})

	buildCommand := root.Command("build", "Build a read-only static documentation site")
	resolveBuildConfig := site.BindFlags(buildCommand.FlagSet)

	buildCommand.Run(func(ctx context.Context) error {
		cfg, err := resolveBuildConfig()
		if err != nil {
			return err
		}

		return site.Run(
			ctx,
			appFS,
			cfg,
			buildCommand.OverriddenValues(),
			stdout,
		)
	})

	pluginsCommand := root.Command("plugins", "Manage static-site plugin dependencies").RequireCommand()

	pluginsSyncCommand := pluginsCommand.Command("sync", "Download and validate declared plugin packages")
	pluginsSyncFile := pluginsSyncCommand.FlagSet.String("file", pluginproject.DefaultFile, "Plugin dependency file").
		NotEmpty().
		Placeholder("FILE")

	pluginsSyncCommand.Run(func(ctx context.Context) error {
		file, err := pluginproject.Load(*pluginsSyncFile.Value())
		if err != nil {
			return err
		}
		resolver, err := pluginproject.NewResolver(plugins.Packages)
		if err != nil {
			return err
		}
		resolved, err := resolver.Resolve(ctx, file.Plugins)
		if err != nil {
			return err
		}
		_, _ = fmt.Fprintf(stdout, "Synced %d plugins from %s\n", len(resolved), *pluginsSyncFile.Value())
		return nil
	})

	pluginsListCommand := pluginsCommand.Command("list", "List declared static-site plugins")
	pluginsListFile := pluginsListCommand.FlagSet.String("file", pluginproject.DefaultFile, "Plugin dependency file").
		NotEmpty().
		Placeholder("FILE")

	pluginsListCommand.Run(func(context.Context) error {
		file, err := pluginproject.Load(*pluginsListFile.Value())
		if err != nil {
			return err
		}
		for _, dependency := range file.Plugins {
			_, _ = fmt.Fprintf(stdout, "%s %s %s@%s%s\n", dependency.ID, dependency.Version, dependency.Repository, dependency.TagPrefix, dependency.Version)
		}
		return nil
	})

	pluginsAddCommand := pluginsCommand.Command("add", "Add a static-site plugin dependency")
	pluginsAddFile := pluginsAddCommand.FlagSet.String("file", pluginproject.DefaultFile, "Plugin dependency file").
		NotEmpty().
		Placeholder("FILE")
	pluginsAddID := pluginsAddCommand.FlagSet.String("id", "", "Plugin ID").Required().NotEmpty().Placeholder("ID")
	pluginsAddRepository := pluginsAddCommand.FlagSet.String("repository", "", "GitHub owner/repository").Required().NotEmpty().Placeholder("OWNER/REPO")
	pluginsAddVersion := pluginsAddCommand.FlagSet.String("plugin-version", "", "Plugin version").Required().NotEmpty().Placeholder("VERSION")
	pluginsAddTagPrefix := pluginsAddCommand.FlagSet.String("tag-prefix", "v", "GitHub release tag prefix").NotEmpty().Placeholder("PREFIX")
	pluginsAddAsset := pluginsAddCommand.FlagSet.String("asset", "", "Release asset base name").Placeholder("NAME")
	pluginsAddCommand.Run(func(ctx context.Context) error {
		dependency, err := pluginproject.NormalizeDependency(pluginproject.Dependency{
			ID:         *pluginsAddID.Value(),
			Repository: *pluginsAddRepository.Value(),
			TagPrefix:  *pluginsAddTagPrefix.Value(),
			Asset:      *pluginsAddAsset.Value(),
			Version:    *pluginsAddVersion.Value(),
		})
		if err != nil {
			return err
		}
		resolver, err := pluginproject.NewResolver(plugins.Packages)
		if err != nil {
			return err
		}
		if _, err := resolver.Resolve(ctx, []pluginproject.Dependency{dependency}); err != nil {
			return err
		}
		if err := pluginproject.Add(*pluginsAddFile.Value(), dependency); err != nil {
			return err
		}
		_, _ = fmt.Fprintf(stdout, "Added %s %s\n", dependency.ID, dependency.Version)
		return nil
	})

	pluginsRemoveCommand := pluginsCommand.Command("remove", "Remove a static-site plugin dependency")
	pluginsRemoveFile := pluginsRemoveCommand.FlagSet.String("file", pluginproject.DefaultFile, "Plugin dependency file").
		NotEmpty().
		Placeholder("FILE")
	pluginsRemoveID := pluginsRemoveCommand.FlagSet.String("id", "", "Plugin ID").Required().NotEmpty().Placeholder("ID")
	pluginsRemoveCommand.Run(func(context.Context) error {
		if err := pluginproject.Remove(*pluginsRemoveFile.Value(), *pluginsRemoveID.Value()); err != nil {
			return err
		}
		_, _ = fmt.Fprintf(stdout, "Removed %s\n", *pluginsRemoveID.Value())
		return nil
	})

	mirrorCommand := root.Command("mirror", "Export PostgreSQL content as a Git-friendly Markdown mirror")
	resolveMirrorConfig := mirror.BindFlags(mirrorCommand.FlagSet)

	mirrorCommand.Run(func(ctx context.Context) error {
		return mirror.Run(ctx, resolveMirrorConfig(), stdout)
	})

	runner, err := root.ParseRunner(args)
	if err != nil {
		switch {
		case tinyflags.IsHelpRequested(err), tinyflags.IsVersionRequested(err):
			_, _ = fmt.Fprint(stdout, err.Error())
			return nil
		case tinyflags.IsCommandRequired(err):
			help, _ := tinyflags.HelpText(err)
			_, _ = fmt.Fprint(stderr, help)

			return nil
		default:
			_, _ = fmt.Fprintln(stderr, err)
			return err
		}
	}

	return runner.Run(ctx)
}
