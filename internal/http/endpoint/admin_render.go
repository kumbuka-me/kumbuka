package endpoint

import (
	"log/slog"
	"net/http"
	"strings"

	"github.com/kumbuka-me/kumbuka/internal/pagecontent"
	"github.com/kumbuka-me/kumbuka/internal/route"
)

// RebuildAdminPageRender rebuilds one page render synchronously for an administrator.
func RebuildAdminPageRender(rebuilds *pagecontent.Rebuilds, logger *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if rebuilds == nil || !rebuilds.Available() {
			http.Error(w, "Page render rebuild is unavailable.", http.StatusServiceUnavailable)
			return
		}
		slug := strings.Trim(strings.TrimSpace(r.PathValue("slug")), "/")
		if err := rebuilds.RebuildPage(r.Context(), slug); err != nil {
			writePageProblem(logger, w, err)
			return
		}
		logger.Info("page render rebuilt", "event", "page_render_rebuild_manual", "slug", slug, "actor_id", currentUser(r).ID)
		route.Redirect(w, r, "/admin/pages", http.StatusSeeOther)
	}
}

// QueueAllAdminPageRenders queues a background rebuild of every current page.
func QueueAllAdminPageRenders(rebuilds *pagecontent.Rebuilds, logger *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if rebuilds == nil || !rebuilds.Available() {
			http.Error(w, "Page render rebuild is unavailable.", http.StatusServiceUnavailable)
			return
		}
		rebuilds.QueueAll("administrator requested all pages")
		logger.Info("page render rebuild queued", "event", "page_render_rebuild_queued", "actor_id", currentUser(r).ID)
		route.Redirect(w, r, "/admin/pages", http.StatusSeeOther)
	}
}

// FlushPendingAdminPageRenders starts a rebuild only when a deferred render-affecting change marked the cache dirty.
func FlushPendingAdminPageRenders(rebuilds *pagecontent.Rebuilds) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		if rebuilds == nil || !rebuilds.Available() {
			http.Error(w, "Page render rebuild is unavailable.", http.StatusServiceUnavailable)
			return
		}
		rebuilds.FlushPending()
		w.WriteHeader(http.StatusNoContent)
	}
}
