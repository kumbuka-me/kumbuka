package endpoint

import (
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	appusers "github.com/kumbuka-me/kumbuka/internal/application/users"
	httpresponse "github.com/kumbuka-me/kumbuka/internal/http/response"
	"github.com/kumbuka-me/kumbuka/internal/webview"
	"github.com/kumbuka-me/kumbuka/pkg/domain"
)

// AdminUsers renders user roles and group memberships.
func AdminUsers(
	viewDataUseCases viewDataService,
	userUseCases adminUserOverviewService,
	groupUseCases groupReader,
	views *webview.Views,
) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		layout, err := administrationData(r, viewDataUseCases, views, "Users", "users")
		data := webview.AdminUsersView{Layout: layout}
		if err != nil {
			httpresponse.InternalServerError(views.Logger(), w, err)
			return
		}

		users, err := userUseCases.Users(r.Context())
		if err != nil {
			httpresponse.InternalServerError(views.Logger(), w, err)
			return
		}

		groups, err := groupUseCases.Groups(r.Context())
		if err != nil {
			httpresponse.InternalServerError(views.Logger(), w, err)
			return
		}

		identities, err := userUseCases.OIDCIdentities(r.Context())
		if err != nil {
			httpresponse.InternalServerError(views.Logger(), w, err)
			return
		}

		identitiesByUser := make(map[int64][]domain.OIDCIdentity)

		for _, identity := range identities {
			identitiesByUser[identity.UserID] = append(identitiesByUser[identity.UserID], identity)
		}
		for index := range users {
			users[index].OIDCIdentities = identitiesByUser[users[index].User.ID]
		}

		pendingIdentities, err := userUseCases.PendingOIDCIdentities(r.Context())
		if err != nil {
			httpresponse.InternalServerError(views.Logger(), w, err)
			return
		}

		identityCount := len(identities)
		data.AdminUsers = users
		data.Groups = groups
		data.PendingOIDCIdentities = pendingIdentities
		data.OIDCIdentityCount = identityCount

		views.Render(w, "admin_users", data)
	}
}

// UpdateAdminUser updates one user's role, group memberships, and optional recovery login state.
func UpdateAdminUser(
	userUseCases userAccountWriter,
	views *webview.Views,
	logger *slog.Logger,
) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		admin := currentUser(r)
		userID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
		if err != nil || userID <= 0 {
			httpresponse.Problem(w,
				http.StatusBadRequest,
				"User validation failed.",
				httpresponse.NewFieldProblem("user_id", "Choose a valid user."),
			)
			return
		}
		if err := r.ParseForm(); err != nil {
			httpresponse.Problem(w, http.StatusBadRequest, "Invalid user form.")
			return
		}

		role := r.FormValue("role")
		enabled := r.FormValue("account_enabled") == "on"

		groupIDs := make([]int64, 0, len(r.Form["group_id"]))

		for _, value := range r.Form["group_id"] {
			groupID, err := strconv.ParseInt(value, 10, 64)
			if err != nil || groupID <= 0 {
				httpresponse.Problem(w,
					http.StatusBadRequest,
					"Group validation failed.",
					httpresponse.NewFieldProblem("group_id", "Choose a valid group."),
				)
				return
			}

			groupIDs = append(groupIDs, groupID)
		}

		password := r.FormValue("local_password")
		updateLocalCredential := r.FormValue("update_local_credential") == "true"
		if problems := localPasswordValidationProblems(
			password,
			r.FormValue("local_password_confirm"),
			"local_password",
			"local_password_confirm",
			false,
		); len(problems) > 0 {
			httpresponse.Problem(w, http.StatusUnprocessableEntity, "Local login validation failed.", problems...)
			return
		}

		if err := userUseCases.UpdateAccount(r.Context(), appusers.UserUpdateInput{
			UserID: userID, Actor: admin, Role: role, Enabled: enabled, GroupIDs: groupIDs,
			Password: password, UpdateLocalCredential: updateLocalCredential,
			LocalCredentialEnabled: r.FormValue("local_credential_enabled") == "on",
			AuthModeOverride:       views.Runtime().AuthModeOverride,
		}); err != nil {
			writeAdminProblem(logger, w, err, "User")
			return
		}

		http.Redirect(w, r, "/admin/users", http.StatusSeeOther)
	}
}

// RevokeAdminUserSessions signs an account out of local and OIDC browser sessions.
func RevokeAdminUserSessions(userUseCases userManagementService, logger *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		admin := currentUser(r)
		userID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
		if err != nil || userID <= 0 {
			httpresponse.Problem(w, http.StatusBadRequest, "Invalid user.")
			return
		}
		if err := userUseCases.RevokeUserSessions(r.Context(), userID, admin.ID); err != nil {
			writeAdminProblem(logger, w, err, "User sessions")
			return
		}

		http.Redirect(w, r, "/admin/users", http.StatusSeeOther)
	}
}

// ApprovePendingOIDCIdentity creates a Kumbuka account for one verified identity request.
func ApprovePendingOIDCIdentity(userUseCases oidcIdentityService, logger *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		admin := currentUser(r)
		pendingID, err := pendingOIDCIdentityID(r)
		if err != nil {
			httpresponse.Problem(w, http.StatusBadRequest, "Invalid identity request.")
			return
		}

		_, err = userUseCases.ApprovePendingOIDCIdentity(r.Context(), pendingID, admin.ID)
		if err != nil {
			writeAdminProblem(logger, w, err, "Identity request")
			return
		}

		http.Redirect(w, r, "/admin/users#pending-identities", http.StatusSeeOther)
	}
}

// LinkPendingOIDCIdentity replaces an existing user's issuer binding with a verified identity request.
func LinkPendingOIDCIdentity(userUseCases oidcIdentityService, logger *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		admin := currentUser(r)
		pendingID, err := pendingOIDCIdentityID(r)
		if err != nil {
			httpresponse.Problem(w, http.StatusBadRequest, "Invalid identity request.")
			return
		}
		if err := r.ParseForm(); err != nil {
			httpresponse.Problem(w, http.StatusBadRequest, "Invalid identity link form.")
			return
		}

		userID, err := strconv.ParseInt(r.FormValue("user_id"), 10, 64)
		if err != nil || userID <= 0 {
			httpresponse.Problem(w,
				http.StatusUnprocessableEntity,
				"Identity link validation failed.",
				httpresponse.NewFieldProblem("user_id", "Choose an existing Kumbuka user."),
			)
			return
		}

		_, err = userUseCases.LinkPendingOIDCIdentity(r.Context(), pendingID, userID, admin.ID)
		if err != nil {
			writeAdminProblem(logger, w, err, "Identity request")
			return
		}

		http.Redirect(w, r, "/admin/users#pending-identities", http.StatusSeeOther)
	}
}

// RejectPendingOIDCIdentity blocks one verified identity request until an administrator reopens it.
func RejectPendingOIDCIdentity(userUseCases oidcIdentityService, logger *slog.Logger) http.HandlerFunc {
	return pendingOIDCIdentityStatusHandler(userUseCases, logger, true)
}

// ReopenPendingOIDCIdentity returns a rejected identity request to the pending queue.
func ReopenPendingOIDCIdentity(userUseCases oidcIdentityService, logger *slog.Logger) http.HandlerFunc {
	return pendingOIDCIdentityStatusHandler(userUseCases, logger, false)
}

// pendingOIDCIdentityStatusHandler updates one administrator decision.
func pendingOIDCIdentityStatusHandler(
	userUseCases oidcIdentityService,
	logger *slog.Logger,
	rejected bool,
) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		admin := currentUser(r)
		pendingID, err := pendingOIDCIdentityID(r)
		if err != nil {
			httpresponse.Problem(w, http.StatusBadRequest, "Invalid identity request.")
			return
		}
		if err := userUseCases.SetPendingOIDCIdentityRejected(
			r.Context(),
			pendingID,
			rejected,
			admin.ID,
		); err != nil {
			writeAdminProblem(logger, w, err, "Identity request")
			return
		}

		http.Redirect(w, r, "/admin/users#pending-identities", http.StatusSeeOther)
	}
}

// pendingOIDCIdentityID parses the pending identity identifier from the route.
func pendingOIDCIdentityID(r *http.Request) (pendingID int64, err error) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		return 0, errors.New("invalid pending OIDC identity")
	}

	return id, nil
}

// RemoveAdminOIDCIdentity disconnects one active OIDC binding from a Kumbuka account.
func RemoveAdminOIDCIdentity(userUseCases oidcIdentityService, logger *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		admin := currentUser(r)
		userID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
		if err != nil || userID <= 0 {
			httpresponse.Problem(w, http.StatusBadRequest, "Invalid user identifier.")
			return
		}
		if err := r.ParseForm(); err != nil {
			httpresponse.Problem(w, http.StatusBadRequest, "Invalid identity form.")
			return
		}

		issuer := strings.TrimSpace(r.FormValue("issuer"))
		subject := strings.TrimSpace(r.FormValue("subject"))
		if issuer == "" || subject == "" {
			httpresponse.Problem(w, http.StatusBadRequest, "Invalid OIDC identity.")
			return
		}
		if err := userUseCases.RemoveOIDCIdentity(
			r.Context(),
			userID,
			issuer,
			subject,
			admin.ID,
		); err != nil {
			writeAdminProblem(logger, w, err, "OIDC identity")
			return
		}

		http.Redirect(w, r, "/admin/users#oidc-identities", http.StatusSeeOther)
	}
}

// SearchAdminUsers returns user matches for live administrator pickers.
func SearchAdminUsers(userUseCases userDirectoryService, logger *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		query := strings.TrimSpace(r.URL.Query().Get("q"))
		if len([]rune(query)) < 2 {
			httpresponse.Respond(w, http.StatusOK, []domain.User{})
			return
		}

		users, err := userUseCases.SearchUsers(r.Context(), query, 20)
		if err != nil {
			httpresponse.InternalServerError(logger, w, err)
			return
		}

		httpresponse.Respond(w, http.StatusOK, jsonSlice(users))
	}
}
