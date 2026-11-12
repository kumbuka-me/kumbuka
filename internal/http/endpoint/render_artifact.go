package endpoint

import (
	"context"
	"net/http"
	"strconv"
	"strings"

	"github.com/kumbuka-me/kumbuka/internal/http/auth"
	"github.com/kumbuka-me/kumbuka/pkg/ascii"
	"github.com/kumbuka-me/kumbuka/pkg/domain"
	"github.com/kumbuka-me/sdk/renderartifact"
)

// PageRenderArtifact delivers passive SVGs only after authorizing access to the owning page.
func PageRenderArtifact(pages interface {
	PageByIDFor(context.Context, domain.User, int64) (domain.Page, error)
}, store pageSVGArtifactStore,
) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := strconv.ParseInt(r.PathValue("pageID"), 10, 64)
		digest := strings.TrimSuffix(r.PathValue("digest"), ".svg")
		if err != nil || id <= 0 || !validArtifactDigest(digest) {
			http.NotFound(w, r)
			return
		}

		user, _ := auth.User(r)
		if _, err := pages.PageByIDFor(r.Context(), user, id); err != nil {
			http.NotFound(w, r)
			return
		}
		data, found, err := store.GetPageSVG(r.Context(), id, digest)
		if err != nil {
			http.Error(w, "artifact unavailable", http.StatusInternalServerError)
			return
		}
		if !found {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", renderartifact.SVGMediaType)
		w.Header().Set("Content-Security-Policy", "default-src 'none'; style-src 'unsafe-inline'; sandbox")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Cache-Control", "private, max-age=3600")
		_, _ = w.Write(data)
	}
}

// validArtifactDigest accepts only a canonical SHA-256 hex string.
func validArtifactDigest(value string) bool {
	if len(value) != 64 {
		return false
	}

	for i := 0; i < len(value); i++ {
		c := value[i]
		if ascii.IsHexDigit(c) && !ascii.IsUppercaseLetter(c) {
			continue
		}
		return false
	}

	return true
}
