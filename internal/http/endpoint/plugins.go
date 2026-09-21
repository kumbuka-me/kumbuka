package endpoint

import (
	"io/fs"
	"mime"
	"net/http"
	"path"
	"regexp"

	"github.com/kumbuka-me/kumbuka/pkg/plugin"
	"github.com/kumbuka-me/kumbuka/pkg/pluginbrowser"
)

// PluginAssets serves one validated browser asset from an enabled plugin.
func PluginAssets(manager *plugin.Manager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		if manager == nil {
			http.NotFound(w, r)
			return
		}
		name := r.PathValue("asset")
		// Asset() also validates the complete path, including backslashes and '..'.
		data, err := manager.BrowserAsset(r.PathValue("pluginID"), r.PathValue("digest"), name)
		if err != nil {
			http.NotFound(w, r)
			return
		}
		contentType := mime.TypeByExtension(path.Ext(name))
		if contentType == "" {
			contentType = "application/octet-stream"
		}
		w.Header().Set("Content-Type", contentType)
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Content-Security-Policy", "default-src 'none'; sandbox")
		_, _ = w.Write(data)
	}
}

// PluginPreview serves static package documentation without enabling or executing the plugin. The route is registered behind browser authentication and administrator authorization.
func PluginPreview(manager *plugin.Manager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "private, no-store")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Content-Security-Policy", "default-src 'none'; sandbox")
		if manager == nil {
			http.NotFound(w, r)
			return
		}

		data, err := manager.PluginPreview(r.PathValue("pluginID"))
		if err != nil {
			http.NotFound(w, r)
			return
		}

		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write(data)
	}
}

var browserHost = regexp.MustCompile(`^[a-zA-Z0-9.\[\]:_-]+$`)

// PluginFrame serves the isolated frame used to execute one plugin browser module.
func PluginFrame(manager *plugin.Manager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		if manager == nil || !browserHost.MatchString(r.Host) {
			http.NotFound(w, r)
			return
		}
		for _, module := range manager.BrowserModules() {
			if !matchesPluginFrameRequest(r, module) {
				continue
			}
			data, policy, err := pluginbrowser.Frame("/plugins", "/plugins/runtime.js", []string{"http://" + r.Host, "https://" + r.Host}, module)
			if err != nil {
				http.Error(w, "Plugin frame unavailable", 500)
				return
			}
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.Header().Set("Content-Security-Policy", policy+"; frame-ancestors 'self'")
			w.Header().Set("X-Frame-Options", "SAMEORIGIN")
			w.Header().Set("Referrer-Policy", "no-referrer")
			_, _ = w.Write(data)
			return
		}
		http.NotFound(w, r)
	}
}

// matchesPluginFrameRequest reports whether a browser module owns the requested immutable frame path.
func matchesPluginFrameRequest(r *http.Request, module plugin.BrowserContribution) bool {
	return module.PluginID == r.PathValue("pluginID") &&
		module.Digest == r.PathValue("digest") &&
		module.ModuleID+".html" == r.PathValue("frame")
}

// PluginBrowserRuntime serves the shared browser bootstrap used by plugin frames.
func PluginBrowserRuntime(appFS fs.FS) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		data, err := fs.ReadFile(appFS, "js/plugins/frame.js")
		if err != nil {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/javascript; charset=utf-8")
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		_, _ = w.Write(data)
	}
}

// PluginPresentationStyles serves core-filtered plugin presentation styles for rendered content.
func PluginPresentationStyles(manager *plugin.Manager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		version := pluginbrowser.PresentationStylesVersion(manager)
		requestedVersion := r.URL.Query().Get("v")

		switch {
		case requestedVersion == "":
			// Keep the unversioned endpoint safe for callers that do not yet use the
			// cache-busting URL emitted by Kumbuka's page templates.
			w.Header().Set("Cache-Control", "no-store")
		case requestedVersion != version:
			w.Header().Set("Cache-Control", "no-store")
			http.NotFound(w, r)
			return
		default:
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		}

		w.Header().Set("Content-Type", "text/css; charset=utf-8")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		_, _ = w.Write([]byte(pluginbrowser.PresentationStyles(manager)))
	}
}
