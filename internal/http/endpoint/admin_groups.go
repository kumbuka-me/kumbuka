package endpoint

import (
	"log/slog"
	"net/http"
	"strconv"

	httpresponse "github.com/kumbuka-me/kumbuka/internal/http/response"
	"github.com/kumbuka-me/kumbuka/internal/webview"
)

// AdminGroups renders group management.
func AdminGroups(
	viewDataUseCases viewDataService,
	groupUseCases groupReader,
	views *webview.Views,
) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		data, err := administrationData(r, viewDataUseCases, views, "Groups", "groups")
		if err != nil {
			httpresponse.InternalServerError(views.Logger(), w, err)
			return
		}

		groups, err := groupUseCases.Groups(r.Context())
		if err != nil {
			httpresponse.InternalServerError(views.Logger(), w, err)
			return
		}

		data.Groups = groups

		views.Render(w, "admin_groups", data)
	}
}

// CreateAdminGroup creates a new user group.
func CreateAdminGroup(groupUseCases groupWriter, logger *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			httpresponse.Problem(w, http.StatusBadRequest, "Invalid group form.")
			return
		}
		if _, err := groupUseCases.CreateGroup(r.Context(), r.FormValue("name")); err != nil {
			writeAdminProblem(logger, w, err, "Group")
			return
		}

		http.Redirect(w, r, "/admin/groups", http.StatusSeeOther)
	}
}

// DeleteAdminGroup deletes one user group.
func DeleteAdminGroup(groupUseCases groupWriter, logger *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
		if err != nil || id <= 0 {
			httpresponse.Problem(w,
				http.StatusBadRequest,
				"Group validation failed.",
				httpresponse.NewFieldProblem("group_id", "Choose a valid group."),
			)
			return
		}
		if err := groupUseCases.DeleteGroup(r.Context(), id); err != nil {
			writeAdminProblem(logger, w, err, "Group")
			return
		}

		http.Redirect(w, r, "/admin/groups", http.StatusSeeOther)
	}
}

// AdminGroupMembers returns the current members of one group.
func AdminGroupMembers(groupUseCases groupReader, logger *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		groupID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
		if err != nil || groupID <= 0 {
			httpresponse.Problem(w,
				http.StatusBadRequest,
				"Invalid group identifier.",
				httpresponse.NewFieldProblem("group_id", "Choose a valid group."),
			)
			return
		}

		members, err := groupUseCases.GroupMembers(r.Context(), groupID)
		if err != nil {
			httpresponse.InternalServerError(logger, w, err)
			return
		}

		httpresponse.Respond(w, http.StatusOK, jsonSlice(members))
	}
}

// groupMemberRequest contains the request payload for group member request.
type groupMemberRequest struct {
	// UserID identifies the user associated with group member request.
	UserID int64 `json:"user_id"`
}

// AddAdminGroupMember assigns one user to a group.
func AddAdminGroupMember(
	groupUseCases groupWriter,
	userUseCases userDirectoryService,
	logger *slog.Logger,
) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		groupID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
		if err != nil || groupID <= 0 {
			httpresponse.Problem(w,
				http.StatusBadRequest,
				"Invalid group identifier.",
				httpresponse.NewFieldProblem("group_id", "Choose a valid group."),
			)
			return
		}

		request, err := decode[groupMemberRequest](w, r)
		if err != nil {
			if tryWriteRequestProblem(w, http.StatusBadRequest, "Invalid member request.", "request", err) {
				return
			}

			httpresponse.InternalServerError(logger, w, err)
			return
		}
		if request.UserID <= 0 {
			httpresponse.Problem(w,
				http.StatusUnprocessableEntity,
				"A user is required.",
				httpresponse.NewFieldProblem("user_id", "Choose a person from the suggestions."),
			)
			return
		}
		if err := groupUseCases.AddGroupMember(r.Context(), groupID, request.UserID); err != nil {
			writeAdminProblem(logger, w, err, "Group or user")
			return
		}

		user, err := userUseCases.User(r.Context(), request.UserID)
		if err != nil {
			writeAdminProblem(logger, w, err, "User")
			return
		}

		httpresponse.Respond(w, http.StatusCreated, user)
	}
}

// RemoveAdminGroupMember removes one user from a group.
func RemoveAdminGroupMember(groupUseCases groupWriter, logger *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		groupID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
		if err != nil || groupID <= 0 {
			httpresponse.Problem(w,
				http.StatusBadRequest,
				"Invalid group identifier.",
				httpresponse.NewFieldProblem("group_id", "Choose a valid group."),
			)
			return
		}

		userID, err := strconv.ParseInt(r.PathValue("userID"), 10, 64)
		if err != nil || userID <= 0 {
			httpresponse.Problem(w,
				http.StatusBadRequest,
				"Invalid user identifier.",
				httpresponse.NewFieldProblem("user_id", "Choose a valid person."),
			)
			return
		}
		if err := groupUseCases.RemoveGroupMember(r.Context(), groupID, userID); err != nil {
			writeAdminProblem(logger, w, err, "Group membership")
			return
		}

		w.WriteHeader(http.StatusNoContent)
	}
}
