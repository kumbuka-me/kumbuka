package site

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/kumbuka-me/kumbuka/internal/domain"
	"github.com/kumbuka-me/kumbuka/web"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStaticBrowserAssetsExcludeBranding(t *testing.T) {
	t.Parallel()

	assert.NotContains(t, staticBrowserAssets, "favicon.svg")
	assert.NotContains(t, staticBrowserAssets, "kumbuka-mark.svg")
	assert.NotContains(t, staticBrowserAssets, "kumbuka.svg")
}

func TestStaticBrowserAssetsIncludeModuleDependencies(t *testing.T) {
	t.Parallel()

	output := t.TempDir()
	require.NoError(t, newBuilder(web.Assets).copyBrowserAssets(output))
	exported := os.DirFS(filepath.Join(output, "assets"))
	// Inspect the actual emitted modules, including side-effect and dynamic imports.
	imports := regexp.MustCompile(`(?:\bfrom\s*|\bimport\s*(?:\(\s*)?)["']([^"']+)["']`)
	for _, name := range staticBrowserAssets {
		if !strings.HasSuffix(name, ".js") {
			continue
		}
		data, err := fs.ReadFile(exported, name)
		require.NoError(t, err)
		for _, match := range imports.FindAllSubmatch(data, -1) {
			dependency := string(match[1])
			if !strings.HasPrefix(dependency, ".") {
				continue
			}
			_, err := fs.Stat(exported, path.Join(path.Dir(name), dependency))
			assert.NoError(t, err, "%s imports missing asset %s", name, dependency)
		}
	}
}

func TestMarkdownFileRoute(t *testing.T) {
	t.Parallel()

	t.Run("root index", func(t *testing.T) {
		t.Parallel()
		assert.Equal(t, "", markdownFileRoute("index.md"))
	})

	t.Run("page", func(t *testing.T) {
		t.Parallel()
		assert.Equal(t, "getting-started", markdownFileRoute("getting-started.md"))
	})

	t.Run("section index", func(t *testing.T) {
		t.Parallel()
		assert.Equal(t, "installation", markdownFileRoute("installation/index.md"))
	})

	t.Run("nested page", func(t *testing.T) {
		t.Parallel()
		assert.Equal(t, "installation/docker", markdownFileRoute("installation/docker.md"))
	})
}

func TestStaticBasePath(t *testing.T) {
	t.Parallel()

	root, err := staticBasePath("")

	require.NoError(t, err)
	assert.Equal(t, "/", root)

	site, err := staticBasePath("https://kumbuka.me/")

	require.NoError(t, err)
	assert.Equal(t, "/", site)

	project, err := staticBasePath("https://example.com/kumbuka/")

	require.NoError(t, err)
	assert.Equal(t, "/kumbuka/", project)
}

func TestRewriteLocalURL(t *testing.T) {
	t.Parallel()

	routes := map[string]string{
		"guide/other.md": "guide/other",
	}

	page, err := rewriteLocalURL("other.md#section", "guide/page.md", routes, "/kumbuka/")

	require.NoError(t, err)
	assert.Equal(t, "/kumbuka/guide/other/#section", page)

	asset, err := rewriteLocalURL("images/example.png", "guide/page.md", routes, "/kumbuka/")

	require.NoError(t, err)
	assert.Equal(t, "/kumbuka/guide/images/example.png", asset)

	_, err = rewriteLocalURL("missing.md", "guide/page.md", routes, "/kumbuka/")

	require.Error(t, err)
}

func TestMarkdownTitle(t *testing.T) {
	t.Parallel()

	title, found := markdownTitle("Intro\n\n# Static sites\n", "static-sites")

	assert.True(t, found)
	assert.Equal(t, "Static sites", title)

	title, found = markdownTitle("No title\n", "getting-started")

	assert.False(t, found)
	assert.Equal(t, "Getting Started", title)
}

func TestDiscoverPagesRejectsDuplicateRoutes(t *testing.T) {
	t.Parallel()

	root := t.TempDir()

	require.NoError(t, os.MkdirAll(filepath.Join(root, "guide"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(root, "index.md"), []byte("# Home\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(root, "guide.md"), []byte("# Guide\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(root, "guide", "index.md"), []byte("# Guide index\n"), 0o644))

	_, err := discoverPages(root)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "map to the same route")
}

func TestHasHomePage(t *testing.T) {
	t.Parallel()

	assert.True(t, hasHomePage([]sourcePage{{Route: ""}, {Route: "guide"}}))
	assert.False(t, hasHomePage([]sourcePage{{Route: "guide"}}))
}

func TestBuilderBuildsReadOnlyStaticSite(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	source := filepath.Join(root, "docs")
	output := filepath.Join(root, "site")

	require.NoError(t, os.MkdirAll(filepath.Join(source, "images"), 0o755))
	require.NoError(t, os.WriteFile(
		filepath.Join(source, "index.md"),
		[]byte("# Home\n\n[Guide](guide.md)\n\n[[Guide]]\n\n![Logo](images/logo.png)\n\n{{subpages title=\"Related pages\"}}\n"),
		0o644,
	))
	require.NoError(t, os.WriteFile(
		filepath.Join(source, "guide.md"),
		[]byte("# Guide\n\n## Details\n\nStatic documentation.\n"),
		0o644,
	))
	require.NoError(t, os.WriteFile(filepath.Join(source, "images", "logo.png"), []byte("png"), 0o644))

	assets := fstest.MapFS{}

	for _, name := range staticBrowserAssets {
		assets[name] = &fstest.MapFile{Data: []byte("asset")}
	}

	config := defaultConfig()
	config.SiteURL = "https://example.com/docs/"
	config.SourceDir = source
	config.OutputDir = output
	config.NavigationStyle = domain.NavigationStyleTopbar
	config.NavigationDensity = domain.NavigationDensityCompact
	config.SidebarWidth = 360
	config.ExternalLinks = []domain.ExternalLink{{
		Label:       "Repository",
		URL:         "https://github.com/kumbuka-me/kumbuka",
		Icon:        "book-lucide",
		Description: "v2.4.1",
	}}

	renderer, _ := testPluginMarkdownRenderer(t, "subpages")
	builder := newBuilder(assets)
	builder.renderer = renderer
	result, err := builder.build(context.Background(), config)

	require.NoError(t, err)
	assert.Equal(t, 2, result.pages)

	home, err := os.ReadFile(filepath.Join(output, "index.html"))

	require.NoError(t, err)
	assert.Contains(t, string(home), `href="/docs/guide/"`)
	assert.Contains(t, string(home), `src="/docs/images/logo.png"`)
	assert.Contains(t, string(home), "Read-only static site")
	assert.Contains(t, string(home), `data-navigation-style="topbar"`)
	assert.Contains(t, string(home), `data-navigation-density="compact"`)
	assert.Contains(t, string(home), `style="--sidebar: 360px"`)
	assert.Contains(t, string(home), `class="desktop-top-navigation"`)
	assert.Contains(t, string(home), `class="top-navigation-page`)
	assert.NotContains(t, string(home), "/edit/")
	assert.NotContains(t, string(home), "/auth/")
	assert.Contains(t, string(home), "Related pages")
	assert.Contains(t, string(home), ">Documentation</span>")
	assert.NotContains(t, string(home), `<link rel="icon"`)
	assert.NotContains(t, string(home), "kumbuka.svg")
	assert.NotContains(t, string(home), "kumbuka-mark.svg")
	assert.Contains(t, string(home), `href="https://github.com/kumbuka-me/kumbuka"`)
	assert.Contains(t, string(home), ">Repository</strong>")
	assert.Contains(t, string(home), ">v2.4.1</small>")

	guide, err := os.ReadFile(filepath.Join(output, "guide", "index.html"))

	require.NoError(t, err)
	assert.Contains(t, string(guide), "Static documentation.")
	assert.NotContains(t, string(guide), ">Guide</h1></div><h1")

	t.Run("404.html", func(t *testing.T) {
		html, err := os.ReadFile(filepath.Join(output, filepath.FromSlash("404.html")))

		require.NoError(t, err)
		assert.Contains(t, string(html), `class="not-found-page"`)
		assert.Contains(t, string(html), "Page not found")
		assert.Contains(t, string(html), "Search documentation")
	})

	t.Run("search/index.html", func(t *testing.T) {
		_, err := os.Stat(filepath.Join(output, filepath.FromSlash("search/index.html")))
		assert.NoError(t, err)
	})

	t.Run("search-index.json", func(t *testing.T) {
		_, err := os.Stat(filepath.Join(output, filepath.FromSlash("search-index.json")))
		assert.NoError(t, err)
	})

	t.Run(".nojekyll", func(t *testing.T) {
		_, err := os.Stat(filepath.Join(output, filepath.FromSlash(".nojekyll")))
		assert.NoError(t, err)
	})

	t.Run("sitemap.xml", func(t *testing.T) {
		_, err := os.Stat(filepath.Join(output, filepath.FromSlash("sitemap.xml")))
		assert.NoError(t, err)
	})

	t.Run("robots.txt", func(t *testing.T) {
		robots, err := os.ReadFile(filepath.Join(output, filepath.FromSlash("robots.txt")))
		require.NoError(t, err)
		assert.Equal(t, "User-agent: *\nAllow: /\nSitemap: https://example.com/docs/sitemap.xml\n", string(robots))
	})

	t.Run("images/logo.png", func(t *testing.T) {
		_, err := os.Stat(filepath.Join(output, filepath.FromSlash("images/logo.png")))
		assert.NoError(t, err)
	})

	t.Run("does not publish bundled branding", func(t *testing.T) {
		for _, name := range []string{"favicon.svg", "kumbuka-mark.svg", "kumbuka.svg"} {
			_, err := os.Stat(filepath.Join(output, "assets", name))
			assert.ErrorIs(t, err, os.ErrNotExist)
		}
	})

	t.Run("assets/js/static.js", func(t *testing.T) {
		_, err := os.Stat(filepath.Join(output, filepath.FromSlash("assets/js/static.js")))
		assert.NoError(t, err)
	})
}

func TestWriteRobots(t *testing.T) {
	t.Parallel()

	t.Run("disallows indexing", func(t *testing.T) {
		t.Parallel()

		config := defaultConfig()
		config.OutputDir = t.TempDir()
		config.RobotsPolicy = domain.RobotsPolicyDisallow

		require.NoError(t, writeRobots(config))

		robots, err := os.ReadFile(filepath.Join(config.OutputDir, "robots.txt"))
		require.NoError(t, err)
		assert.Equal(t, "User-agent: *\nDisallow: /\n", string(robots))
	})

	t.Run("can be disabled", func(t *testing.T) {
		t.Parallel()

		config := defaultConfig()
		config.OutputDir = t.TempDir()
		config.RobotsPolicy = domain.RobotsPolicyNone

		require.NoError(t, writeRobots(config))

		_, err := os.Stat(filepath.Join(config.OutputDir, "robots.txt"))
		assert.ErrorIs(t, err, os.ErrNotExist)
	})
}

func TestValidateWikiLinksRejectsMissingTarget(t *testing.T) {
	t.Parallel()

	page := sourcePage{SourcePath: "index.md", Markdown: "See [[Missing page]]."}
	err := validateWikiLinks(page, map[string]string{"existing": "existing"})

	require.Error(t, err)
	assert.Contains(t, err.Error(), `unresolved wiki link "missing-page"`)
}

func TestBuildConfiguredBranding(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	contentDir := filepath.Join(root, "content")
	assetsDir := filepath.Join(root, "assets")
	outputDir := filepath.Join(root, "site")
	require.NoError(t, os.MkdirAll(filepath.Join(contentDir, "assets"), 0o755))
	require.NoError(t, os.MkdirAll(assetsDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(contentDir, "index.md"), []byte("# Home"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(assetsDir, "never.svg"), []byte("custom logo"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(contentDir, "assets", "favicon.svg"), []byte("custom favicon"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(contentDir, "favicon.ico"), []byte("custom ico"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(assetsDir, "extra.txt"), []byte("custom asset"), 0o644))

	configFile := filepath.Join(root, "kumbuka-site.toml")
	configSource := fmt.Sprintf(`site_url = "https://example.com/never/"
source_dir = %q
output_dir = %q
logo = "assets/never.svg"
favicon = "content/assets/favicon.svg"
favicon_ico = "content/favicon.ico"
assets_dir = "assets"
`, contentDir, outputDir)
	require.NoError(t, os.WriteFile(configFile, []byte(configSource), 0o644))

	config, err := loadConfig(configFile, true)
	require.NoError(t, err)
	builder := newBuilder(web.Assets)
	builder.renderer = testMarkdownRenderer(t)
	_, err = builder.build(context.Background(), config)
	require.NoError(t, err)

	t.Run("home branding", func(t *testing.T) {
		html, err := os.ReadFile(filepath.Join(config.OutputDir, "index.html"))
		require.NoError(t, err)
		assert.Contains(t, string(html), `src="/never/assets/never.svg"`)
		assert.Contains(t, string(html), `href="/never/assets/favicon.svg"`)
		assert.Contains(t, string(html), `href="/never/favicon.ico"`)
	})

	t.Run("search branding", func(t *testing.T) {
		html, err := os.ReadFile(filepath.Join(config.OutputDir, "search", "index.html"))
		require.NoError(t, err)
		assert.Contains(t, string(html), `src="/never/assets/never.svg"`)
		assert.Contains(t, string(html), `href="/never/assets/favicon.svg"`)
		assert.Contains(t, string(html), `href="/never/favicon.ico"`)
	})

	t.Run("not found branding", func(t *testing.T) {
		html, err := os.ReadFile(filepath.Join(config.OutputDir, "404.html"))
		require.NoError(t, err)
		assert.Contains(t, string(html), `src="/never/assets/never.svg"`)
		assert.Contains(t, string(html), `href="/never/assets/favicon.svg"`)
		assert.Contains(t, string(html), `href="/never/favicon.ico"`)
	})

	t.Run("logo copied", func(t *testing.T) {
		data, err := os.ReadFile(filepath.Join(config.OutputDir, "assets", "never.svg"))
		require.NoError(t, err)
		assert.Equal(t, "custom logo", string(data))
	})

	t.Run("favicon copied", func(t *testing.T) {
		data, err := os.ReadFile(filepath.Join(config.OutputDir, "assets", "favicon.svg"))
		require.NoError(t, err)
		assert.Equal(t, "custom favicon", string(data))
	})

	t.Run("ICO fallback copied", func(t *testing.T) {
		data, err := os.ReadFile(filepath.Join(config.OutputDir, "favicon.ico"))
		require.NoError(t, err)
		assert.Equal(t, "custom ico", string(data))
	})

	t.Run("assets directory copied", func(t *testing.T) {
		data, err := os.ReadFile(filepath.Join(config.OutputDir, "assets", "extra.txt"))
		require.NoError(t, err)
		assert.Equal(t, "custom asset", string(data))
	})

	// Invalid branding must not erase the previously generated site.
	config.Logo = filepath.Join(root, "missing.svg")
	_, err = builder.build(context.Background(), config)
	require.ErrorContains(t, err, "logo")
	_, err = os.Stat(filepath.Join(config.OutputDir, "index.html"))
	require.NoError(t, err)
}
