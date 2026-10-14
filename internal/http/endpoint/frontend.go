package endpoint

import (
	"io/fs"
	"net/http"
	"strings"

	httpkitassets "github.com/containeroo/httpkit/assets"
	httpprefix "github.com/containeroo/httpkit/prefix"
)

// Assets serves embedded browser assets with content-versioned cache semantics.
func Assets(appFS fs.FS) http.Handler {
	files := httpkitassets.Static(httpkitassets.StaticConfig{
		Files: appFS,
		// Every path accepted by Assets contains a content-version segment.
		ImmutablePrefix: "/",
	})
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path, ok := assetPath(r.URL.Path)
		if !ok {
			http.NotFound(w, r)
			return
		}

		request := r.Clone(r.Context())
		request.URL.Path = "/" + path

		files.ServeHTTP(w, request)
	})
}

// assetPath removes the required content-version prefix from an asset request path.
func assetPath(requestPath string) (string, bool) {
	requestPath, ok := strings.CutPrefix(requestPath, "/assets/")
	if !ok {
		return "", false
	}
	version, remainder, ok := strings.Cut(requestPath, "/")
	if !validVersionedAssetPath(version, remainder, ok) {
		return "", false
	}
	return remainder, true
}

// validVersionedAssetPath reports whether an asset path contains a non-empty content-version segment and resource path.
func validVersionedAssetPath(version, remainder string, separated bool) bool {
	return separated && strings.HasPrefix(version, "v-") && len(version) > 2 && remainder != ""
}

// ServiceWorker serves the deployment-scoped progressive-web-app worker without long-lived caching.
func ServiceWorker(appFS fs.FS) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		data, err := fs.ReadFile(appFS, "sw.js")
		if err != nil {
			http.NotFound(w, r)
			return
		}

		w.Header().Set("Content-Type", "text/javascript; charset=utf-8")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Service-Worker-Allowed", httpprefix.URLForRequest(r, "/"))

		_, _ = w.Write(data)
	})
}
