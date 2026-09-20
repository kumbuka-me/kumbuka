// A loopback-only, in-memory fixture for the plugin administration browser test.
package main

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/kumbuka-me/kumbuka/internal/http/auth"
	"github.com/kumbuka-me/kumbuka/internal/http/endpoint"
	"github.com/kumbuka-me/kumbuka/internal/http/middleware"
	"github.com/kumbuka-me/kumbuka/internal/webview"
	"github.com/kumbuka-me/kumbuka/pkg/domain"
	"github.com/kumbuka-me/kumbuka/pkg/markdown"
	"github.com/kumbuka-me/kumbuka/pkg/themes"
	"github.com/kumbuka-me/kumbuka/plugins"
	"github.com/kumbuka-me/kumbuka/web"
	"github.com/kumbuka-me/sdk/pluginpackage"
)

// dataLoader groups data used by data loader.
type dataLoader struct {
	// catalog contains the catalog associated with data loader.
	catalog []themes.Theme
}

// Load loads the browser-test view data requested by a endpoint.
func (d dataLoader) Load(_ *http.Request, _ *webview.Views, title string) (webview.Data, error) {
	data, _ := json.Marshal(d.catalog)
	return webview.Data{
		Preferences: domain.DefaultUserPreferences(),
		Themes:      d.catalog,
		Title:       title,
		User: domain.User{
			ID:       1,
			Username: "admin",
			Role:     "admin",
		},
		AssetVersion: "test",
		ActiveTheme:  "Light",
		ThemeData:    template.JS(data),
	}, nil
}

// main runs the browser-test fixture server.
func main() {
	ctx := context.Background()
	tablesArchive, err := plugins.Packages.ReadFile("tables.kumbukaplugin")
	if err != nil {
		panic(err)
	}
	renderer, err := markdown.NewWithPluginPackages(ctx, [][]byte{tablesArchive}, nil)
	if err != nil {
		panic(err)
	}
	defer func() { _ = renderer.Close(ctx) }()
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	catalog, err := themes.Load("")
	if err != nil {
		panic(err)
	}
	views, err := webview.New(
		web.Assets,
		logger,
		"test",
		"test",
		catalog,
		webview.RuntimeInfo{},
	)
	if err != nil {
		panic(err)
	}
	admin := endpoint.NewAdminPlugins(
		renderer.PluginManager(),
		nil,
		dataLoader{catalog: catalog},
		views,
	)
	mux := http.NewServeMux()
	secure := func(fn http.HandlerFunc) http.Handler { return middleware.RequireRole("admin")(fn) }
	mux.Handle("GET /admin/plugins", secure(admin.List))
	mux.Handle("POST /admin/plugins", secure(admin.Install))
	mux.Handle("POST /admin/plugins/{pluginID}/{action}", secure(admin.Action))
	mux.Handle("GET /assets/", endpoint.Assets(web.Assets))
	mux.HandleFunc("GET /plugins/styles.css", endpoint.PluginPresentationStyles(renderer.PluginManager()))
	mux.HandleFunc("GET /fixture/package", func(w http.ResponseWriter, r *http.Request) {
		version := r.URL.Query().Get("version")
		if version != "1.1.0" {
			version = "1.0.0"
		}
		archive, err := fixturePackage(version)
		if err != nil {
			http.Error(w, "fixture failed", 500)
			return
		}
		_, _ = w.Write(archive)
	})
	mux.HandleFunc("GET /fixture/render", func(w http.ResponseWriter, r *http.Request) {
		result, err := renderer.Render("| A | B |\n| --- | --- |\n| one | two |\n")
		if err != nil {
			http.Error(w, "render failed", 500)
			return
		}
		_, _ = w.Write([]byte(result))
	})
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		panic(err)
	}

	handler := middleware.SecurityHeaders()(middleware.RejectCrossSiteWrites(logger)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		role := "admin"
		if r.Header.Get("X-Fixture-Role") != "" {
			role = r.Header.Get("X-Fixture-Role")
		}
		mux.ServeHTTP(w, auth.WithUser(r, domain.User{ID: 1, Role: role}))
	})))

	fmt.Println("http://" + listener.Addr().String())
	server := &http.Server{
		ReadHeaderTimeout: 5 * time.Second,
		Handler:           handler,
	}
	if err := server.Serve(listener); err != nil && err != http.ErrServerClosed {
		panic(err)
	}
}

// fixturePackage builds the plugin package used by the browser-test fixture.
func fixturePackage(version string) ([]byte, error) {
	original, err := plugins.Packages.ReadFile("callouts.kumbukaplugin")
	if err != nil {
		return nil, err
	}
	pkg, err := pluginpackage.Read(original)
	if err != nil {
		return nil, err
	}
	currentVersion := pkg.Manifest().Version
	reader, err := zip.NewReader(bytes.NewReader(original), int64(len(original)))
	if err != nil {
		return nil, err
	}
	var output bytes.Buffer
	writer := zip.NewWriter(&output)
	for _, entry := range reader.File {
		source, err := entry.Open()
		if err != nil {
			return nil, err
		}
		data, err := io.ReadAll(source)
		_ = source.Close()
		if err != nil {
			return nil, err
		}
		if entry.Name == "plugin.yaml" {
			s := strings.ReplaceAll(string(data), "me.kumbuka.callouts", "io.example.browser")
			s = strings.ReplaceAll(s, "name: Callouts", "name: Browser Fixture")
			s = strings.Replace(s, "version: "+currentVersion, "version: "+version, 1)
			data = []byte(s)
		}
		destination, err := writer.Create(entry.Name)
		if err != nil {
			return nil, err
		}
		if _, err := destination.Write(data); err != nil {
			return nil, err
		}
	}
	if err := writer.Close(); err != nil {
		return nil, err
	}
	return output.Bytes(), nil
}
