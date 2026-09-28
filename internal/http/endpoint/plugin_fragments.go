package endpoint

import (
	"errors"
	"log/slog"
	"net/http"
	"regexp"
	"strconv"

	"github.com/kumbuka-me/kumbuka/internal/http/auth"
	httpresponse "github.com/kumbuka-me/kumbuka/internal/http/response"
	md "github.com/kumbuka-me/kumbuka/pkg/markdown"
	"github.com/kumbuka-me/kumbuka/pkg/plugincap"
)

const maxDeferredMacroIndex = 999999

var deferredLocale = regexp.MustCompile(`^[A-Za-z0-9-]{0,35}$`)

// PluginMacroFragment renders one slow, network-backed macro after the page
// shell has loaded. The invocation is always re-derived from authorized saved
// Markdown; browser input cannot supply provider URLs, credentials, or options.
func PluginMacroFragment(
	reports pageReportService,
	navigation navigationService,
	renderer *md.Renderer,
	logger *slog.Logger,
) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		index, err := strconv.Atoi(r.PathValue("index"))
		if err != nil || index < 0 || index > maxDeferredMacroIndex {
			http.NotFound(w, r)
			return
		}
		pluginID, moduleID := r.PathValue("pluginID"), r.PathValue("moduleID")
		if !renderer.ShouldDeferMacro(pluginID, moduleID) {
			http.NotFound(w, r)
			return
		}

		user, _ := auth.User(r)
		page, err := reports.GetPageFor(r.Context(), user, r.PathValue("slug"))
		if err != nil {
			http.NotFound(w, r)
			return
		}
		version := strconv.FormatInt(page.UpdatedAt.UnixNano(), 10)
		if r.URL.Query().Get("v") != version {
			httpresponse.Problem(w, http.StatusConflict, "The page changed before external content loaded.")
			return
		}

		pageNavigation, err := subpageNavigation(r.Context(), navigation, user, page.Slug)
		if err != nil {
			httpresponse.InternalServerError(logger, w, err)
			return
		}
		locale := r.URL.Query().Get("locale")
		if !deferredLocale.MatchString(locale) {
			http.NotFound(w, r)
			return
		}
		html, err := renderer.RenderDeferredMacro(
			page.Markdown,
			index,
			pluginID,
			moduleID,
			md.Slug,
			md.DefaultOptions(),
			md.Functions{
				Context:      r.Context(),
				Locale:       locale,
				PluginUsage:  page.PluginUsage,
				Capabilities: plugincap.Capabilities(reports.Accessible(user), pageNavigation, renderer.IconCatalog()),
			},
		)
		if errors.Is(err, md.ErrDeferredMacroNotFound) {
			http.NotFound(w, r)
			return
		}
		if err != nil {
			httpresponse.InternalServerError(logger, w, err)
			return
		}

		w.Header().Set("Cache-Control", "private, no-store")
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		_, _ = w.Write([]byte(html))
	}
}
