package handler

import (
	"net/http"
	"strings"

	"github.com/kumbuka-me/kumbuka/internal/externalfiles"
	"github.com/kumbuka-me/kumbuka/internal/httpresponse"
	"github.com/kumbuka-me/kumbuka/internal/webview"
)

// AdminExternalFiles manages core-owned approvals outside guest plugin storage.
type AdminExternalFiles struct {
	sources *externalfiles.Service
	data    viewDataService
	views   *webview.Views
}

// NewAdminExternalFiles constructs handlers protected by the administrator routes.
func NewAdminExternalFiles(s *externalfiles.Service, data viewDataService, views *webview.Views) *AdminExternalFiles {
	return &AdminExternalFiles{sources: s, data: data, views: views}
}

// List displays approvals without decrypting or returning credentials.
func (a *AdminExternalFiles) List(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "private, no-store")
	data, err := administrationData(r, a.data, a.views, "External files", "plugins")
	if err != nil {
		httpresponse.InternalServerError(a.views.Logger(), w, err)
		return
	}
	data.ExternalFilesInsecureTLS = a.sources.InsecureTLS()
	data.ExternalSources, err = a.sources.List(r.Context())
	if err != nil {
		httpresponse.InternalServerError(a.views.Logger(), w, err)
		return
	}
	a.views.Render(w, "admin_external_files", data)
}

// Save requires explicit disclosure approval and never echoes submitted secrets.
func (a *AdminExternalFiles) Save(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "private, no-store")
	r.Body = http.MaxBytesReader(w, r.Body, 16<<10)
	if r.ParseForm() != nil || r.PostForm.Get("approve") != "on" {
		http.Error(w, "Confirm approval of this repository for Kumbuka readers.", http.StatusBadRequest)
		return
	}
	v := externalfiles.Source{ID: strings.TrimSpace(r.PostForm.Get("id")), Provider: r.PostForm.Get("provider"), Endpoint: strings.TrimSpace(r.PostForm.Get("endpoint")), Repository: strings.TrimSpace(r.PostForm.Get("repository")), Ref: strings.TrimSpace(r.PostForm.Get("ref")), Enabled: true, PrivateIPs: strings.Fields(r.PostForm.Get("private_ips"))}
	if err := a.sources.Save(r.Context(), v, r.PostForm.Get("token")); err != nil {
		http.Error(w, "Could not save source. Check the source fields and application encryption key.", http.StatusBadRequest)
		return
	}
	a.views.Logger().Info("external source approved", "event", "external_source.approve", "source", v.ID, "actor_id", currentUser(r).ID)
	http.Redirect(w, r, "/admin/plugins/external-files", http.StatusSeeOther)
}

// Delete revokes a source; future renders cannot fetch its content.
func (a *AdminExternalFiles) Delete(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "private, no-store")
	if err := a.sources.Delete(r.Context(), r.PathValue("source")); err != nil {
		http.Error(w, "Could not revoke source.", http.StatusBadRequest)
		return
	}
	a.views.Logger().Info("external source revoked", "event", "external_source.revoke", "source", r.PathValue("source"), "actor_id", currentUser(r).ID)
	http.Redirect(w, r, "/admin/plugins/external-files", http.StatusSeeOther)
}
