package site

import (
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"
)

var staticBrowserAssets = []string{
	"css/app.css",
	"js/static.js",
	"js/theme-init.js",
	"js/core/clipboard.js",
	"js/core/dom.js",
	"js/core/guards.js",
	"js/core/http.js",
	"js/core/theme.js",
	"js/features/markdown.js",
	"js/plugins/loader.js",
	"js/plugins/frame.js",
	"js/features/static-layout.js",
	"js/features/static-page.js",
	"js/features/static-search.js",
}

type brandingData struct {
	LogoURL       string
	FaviconURL    string
	FaviconICOURL string
}

// prepareOutput recreates the output directory and publishes runtime, source, and branding assets.
func (b *builder) prepareOutput(config Config, basePath string) (brandingData, error) {
	if err := recreateDirectory(config.OutputDir); err != nil {
		return brandingData{}, err
	}
	if err := b.copyBuildAssets(config); err != nil {
		return brandingData{}, err
	}

	if err := b.copyPluginAssets(config, basePath); err != nil {
		return brandingData{}, err
	}

	branding, err := publishBranding(config, basePath)
	if err != nil {
		return brandingData{}, err
	}
	if err := os.WriteFile(filepath.Join(config.OutputDir, ".nojekyll"), nil, 0o644); err != nil {
		return brandingData{}, err
	}

	return branding, nil
}

// recreateDirectory replaces one directory with an empty writable directory.
func recreateDirectory(directory string) error {
	if err := os.RemoveAll(directory); err != nil {
		return err
	}

	return os.MkdirAll(directory, 0o755)
}

// copyBuildAssets publishes configured assets, required browser assets, and source files in precedence order.
func (b *builder) copyBuildAssets(config Config) error {
	if err := copyConfiguredAssets(config.AssetsDir, config.OutputDir); err != nil {
		return err
	}
	if err := b.copyBrowserAssets(config.OutputDir); err != nil {
		return err
	}
	return copySourceAssets(config.SourceDir, config.OutputDir)
}

// copyConfiguredAssets publishes the optional asset directory below the generated assets path.
func copyConfiguredAssets(sourceDir, outputDir string) error {
	if sourceDir == "" {
		return nil
	}

	if err := copySourceAssets(sourceDir, filepath.Join(outputDir, "assets")); err != nil {
		return fmt.Errorf("copy assets_dir: %w", err)
	}

	return nil
}

// copyBrowserAssets publishes only the browser runtime files required by static pages.
func (b *builder) copyBrowserAssets(outputDir string) error {
	for _, name := range staticBrowserAssets {
		data, err := fs.ReadFile(b.appFS, name)
		if err != nil {
			return fmt.Errorf("read browser asset %s: %w", name, err)
		}

		destination := filepath.Join(outputDir, "assets", filepath.FromSlash(name))
		if err := writeFile(destination, data); err != nil {
			return err
		}
	}

	return nil
}

// copySourceAssets recursively copies non-Markdown source files while skipping hidden directories.
func copySourceAssets(sourceDir, outputDir string) error {
	return filepath.WalkDir(sourceDir, func(filename string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if filename == sourceDir {
			return nil
		}
		if entry.IsDir() {
			if strings.HasPrefix(entry.Name(), ".") {
				return filepath.SkipDir
			}
			return nil
		}
		if strings.EqualFold(filepath.Ext(entry.Name()), ".md") {
			return nil
		}

		relative, err := filepath.Rel(sourceDir, filename)
		if err != nil {
			return err
		}

		return copyFile(filename, filepath.Join(outputDir, relative))
	})
}

// publishBranding resolves and publishes only branding files explicitly configured by the user.
func publishBranding(config Config, basePath string) (brandingData, error) {
	branding := brandingData{}

	logoURL, err := publishConfiguredFile(config, config.Logo)
	if err != nil {
		return brandingData{}, fmt.Errorf("publish logo: %w", err)
	}
	if logoURL != "" {
		branding.LogoURL = basePath + logoURL
	}

	faviconURL, err := publishConfiguredFile(config, config.Favicon)
	if err != nil {
		return brandingData{}, fmt.Errorf("publish favicon: %w", err)
	}
	if faviconURL != "" {
		branding.FaviconURL = basePath + faviconURL
	}

	faviconICOURL, err := publishConfiguredFile(config, config.FaviconICO)
	if err != nil {
		return brandingData{}, fmt.Errorf("publish favicon_ico: %w", err)
	}
	if faviconICOURL != "" {
		branding.FaviconICOURL = basePath + faviconICOURL
	}

	return branding, nil
}

// publishConfiguredFile publishes one configured file while preserving its natural public path when possible.
func publishConfiguredFile(config Config, filename string) (string, error) {
	if filename == "" {
		return "", nil
	}

	publicPath, err := configuredPublicPath(config, filename)
	if err != nil {
		return "", err
	}
	if err := copyFile(filename, filepath.Join(config.OutputDir, filepath.FromSlash(publicPath))); err != nil {
		return "", err
	}

	return publicPath, nil
}

// configuredPublicPath derives the generated public path for one configured file.
func configuredPublicPath(config Config, filename string) (string, error) {
	if relative, found, err := relativeFilePath(config.SourceDir, filename); err != nil {
		return "", err
	} else if found {
		return relative, nil
	}

	if relative, found, err := relativeFilePath(config.AssetsDir, filename); err != nil {
		return "", err
	} else if found {
		return path.Join("assets", relative), nil
	}

	return path.Join("assets", filepath.Base(filename)), nil
}

// relativeFilePath returns a slash-separated path when filename is contained by root.
func relativeFilePath(root, filename string) (string, bool, error) {
	if root == "" {
		return "", false, nil
	}

	absoluteRoot, err := filepath.Abs(root)
	if err != nil {
		return "", false, err
	}
	absoluteFile, err := filepath.Abs(filename)
	if err != nil {
		return "", false, err
	}

	relative, err := filepath.Rel(absoluteRoot, absoluteFile)
	if err != nil {
		return "", false, err
	}
	if relative == "." || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return "", false, nil
	}

	return filepath.ToSlash(relative), true, nil
}

// copyFile copies one filesystem file and creates its destination directory.
func copyFile(source, destination string) error {
	if err := os.MkdirAll(filepath.Dir(destination), 0o755); err != nil {
		return err
	}

	input, err := os.Open(source)
	if err != nil {
		return err
	}

	output, err := os.Create(destination)
	if err != nil {
		_ = input.Close()
		return err
	}

	_, copyErr := io.Copy(output, input)
	inputCloseErr := input.Close()
	outputCloseErr := output.Close()
	if copyErr != nil {
		return copyErr
	}
	if inputCloseErr != nil {
		return inputCloseErr
	}

	return outputCloseErr
}

// writeFile writes one generated file and creates its destination directory.
func writeFile(filename string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(filename), 0o755); err != nil {
		return err
	}

	return os.WriteFile(filename, data, 0o644)
}
