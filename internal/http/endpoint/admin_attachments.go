package endpoint

import (
	"errors"
	"net/http"
	"strconv"

	httpresponse "github.com/kumbuka-me/kumbuka/internal/http/response"
	"github.com/kumbuka-me/kumbuka/internal/webview"
	"github.com/kumbuka-me/kumbuka/pkg/domain"
)

// DeleteAdminAttachment removes one attachment from the administrator attachment screen.
func DeleteAdminAttachment(mediaUseCases attachmentAdminService, views *webview.Views) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
		if err != nil || id <= 0 {
			http.NotFound(w, r)
			return
		}

		if err := mediaUseCases.DeleteAttachment(r.Context(), id, currentUser(r)); err != nil {
			if errors.Is(err, domain.ErrNotFound) {
				http.NotFound(w, r)
				return
			}
			httpresponse.InternalServerError(views.Logger(), w, err)
			return
		}

		http.Redirect(w, r, "/admin/attachments", http.StatusSeeOther)
	}
}
