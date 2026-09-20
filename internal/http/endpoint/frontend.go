package endpoint

import (
	"io/fs"
	"net/http"
	"strings"
)

// Assets serves embedded browser assets with content-versioned cache semantics.
func Assets(appFS fs.FS) http.Handler {
	files := http.FileServer(http.FS(appFS))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path, ok := assetPath(r.URL.Path)
		if !ok {
			http.NotFound(w, r)
			return
		}

		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")

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
	if !ok || !strings.HasPrefix(version, "v-") || len(version) <= 2 || remainder == "" {
		return "", false
	}
	return remainder, true
}

// ServiceWorker serves the root-scoped progressive-web-app worker without long-lived caching.
func ServiceWorker(appFS fs.FS) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		data, err := fs.ReadFile(appFS, "sw.js")
		if err != nil {
			http.NotFound(w, r)
			return
		}

		w.Header().Set("Content-Type", "text/javascript; charset=utf-8")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Service-Worker-Allowed", "/")

		_, _ = w.Write(data)
	})
}
