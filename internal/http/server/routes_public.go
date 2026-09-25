package server

import (
	"net/http"

	"github.com/kumbuka-me/kumbuka/internal/http/endpoint"
)

// registerPublicRoutes registers unauthenticated assets, authentication, setup, and discovery endpoints.
func registerPublicRoutes(mux *http.ServeMux, config Config) {
	browserAuth := config.BrowserAuth
	renderer := config.Renderer

	mux.HandleFunc("GET /plugins/styles.css", endpoint.PluginPresentationStyles(renderer.PluginManager()))
	mux.HandleFunc("GET /plugins/runtime.js", endpoint.PluginBrowserRuntime(config.Assets))
	mux.HandleFunc("GET /plugins/{pluginID}/{digest}/assets/{asset...}", endpoint.PluginAssets(renderer.PluginManager()))
	mux.HandleFunc("GET /plugins/{pluginID}/{digest}/frames/{frame}", endpoint.PluginFrame(renderer.PluginManager()))
	mux.Handle("GET /robots.txt", endpoint.Robots(config.Settings, config.Views, config.Logger))
	mux.Handle("GET /sitemap.xml", endpoint.Sitemap(config.Settings, config.PageDirectory, config.Views, config.Logger))
	mux.Handle("GET /assets/", endpoint.Assets(config.Assets))
	mux.Handle("GET /sw.js", endpoint.ServiceWorker(config.Assets))
	mux.Handle("GET /brand/logo", endpoint.BrandLogo(config.Settings, config.Assets, config.Logger))
	mux.Handle("GET /auth/login", browserAuth.Login)

	localLogin := endpoint.LocalLogin(config.Settings, config.System, browserAuth, config.Views)
	mux.Handle("GET /auth/local", localLogin)
	mux.Handle("POST /auth/local", localLogin)

	setup := endpoint.Setup(config.Settings, config.System, browserAuth, config.Views)
	mux.Handle("GET /setup", setup)
	mux.Handle("POST /setup", setup)

	if browserAuth.Callback != nil {
		mux.Handle("GET /auth/callback", browserAuth.Callback)
	}
}
