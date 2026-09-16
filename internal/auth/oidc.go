package auth

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/kumbuka-me/kumbuka/internal/httpresponse"
	"github.com/kumbuka-me/kumbuka/pkg/domain"
	"golang.org/x/oauth2"
)

const (
	oidcSessionCookie = "kumbuka_session"
	oidcStateCookie   = "kumbuka_state"
	// oidcLegacyNextCookie is cleared after callbacks for installations upgraded from the old two-cookie login flow.
	oidcLegacyNextCookie = "kumbuka_next"

	oidcLoginStateTTL = 10 * time.Minute
	oidcSessionTTL    = 12 * time.Hour
)

// OIDC authenticates browser sessions and handles the OIDC authorization flow.
type OIDC struct {
	// repository persists and resolves authenticated OIDC users.
	repository oidcRepository
	// verifier validates OIDC identity tokens.
	verifier *oidc.IDTokenVerifier
	// oauth contains the OIDC authorization-code client configuration.
	oauth *oauth2.Config
	// secret signs and verifies application cookies.
	secret []byte
	// issuer is the verified provider namespace used with OIDC subjects.
	issuer string
	// publicURL is the externally visible Kumbuka base URL.
	publicURL string
	// groupClaim is the top-level ID-token claim containing external groups.
	groupClaim string
	// groupSync enables mapped group membership synchronization at login.
	groupSync bool
	// groupsAuthoritative removes mapped memberships absent from the current claim.
	groupsAuthoritative bool
	// groupMappings maps external claim values to Kumbuka groups.
	groupMappings []domain.OIDCGroupMapping
	// adminGroup is the external group value that elevates the current session.
	adminGroup string
}

// claims contains the OIDC identity claims used to create a Kumbuka user.
type claims struct {
	// Email is the user email returned by the identity provider.
	Email string `json:"email"`
	// PreferredUsername is the preferred login name returned by the identity provider.
	PreferredUsername string `json:"preferred_username"`
	// Name is the display name returned by the identity provider.
	Name string `json:"name"`
}

// session contains only the stable OIDC identity needed to resolve a Kumbuka user.
type session struct {
	// Issuer namespaces the OIDC subject.
	Issuer string `json:"iss"`
	// Subject is the stable user identifier assigned by the issuer.
	Subject string `json:"sub"`
	// Expires is the Unix timestamp after which the session is invalid.
	Expires int64 `json:"x"`
	// Version invalidates the cookie after an administrator revokes sessions.
	Version int64 `json:"v"`
}

// loginState contains the signed OIDC state and its expiration.
type loginState struct {
	// State is the saved OIDC state value.
	State string `json:"s"`
	// Verifier binds the authorization code to this browser login.
	Verifier string `json:"p"`
	// Next is the local destination bound to this login attempt.
	Next string `json:"n,omitempty"`
	// Expires is the Unix expiration timestamp for the saved state.
	Expires int64 `json:"x"`
}

// NewOIDC creates an OIDC authenticator and authorization-flow handler.
func NewOIDC(
	ctx context.Context,
	config OIDCConfig,
	repository oidcRepository,
) (*OIDC, error) {
	provider, err := oidc.NewProvider(ctx, config.Issuer)
	if err != nil {
		return nil, err
	}

	return &OIDC{
		repository:          repository,
		verifier:            provider.Verifier(&oidc.Config{ClientID: config.ClientID}),
		secret:              []byte(config.SessionSecret),
		issuer:              strings.TrimSpace(config.Issuer),
		publicURL:           config.PublicURL,
		groupClaim:          strings.TrimSpace(config.GroupClaim),
		groupSync:           config.GroupSync,
		groupsAuthoritative: config.GroupsAuthoritative,
		groupMappings:       slices.Clone(config.GroupMappings),
		adminGroup:          strings.TrimSpace(config.AdminGroup),
		oauth: &oauth2.Config{
			ClientID:     config.ClientID,
			ClientSecret: config.ClientSecret,
			Endpoint:     provider.Endpoint(),
			RedirectURL:  strings.TrimRight(config.PublicURL, "/") + "/auth/callback",
			Scopes:       []string{oidc.ScopeOpenID, "profile", "email"},
		},
	}, nil
}

// Authenticate resolves the authenticated OIDC browser session by issuer and subject.
func (o *OIDC) Authenticate(r *http.Request) (domain.User, error) {
	var current session

	if err := o.decodeCookie(r, oidcSessionCookie, &current); err != nil {
		return domain.User{}, ErrUnauthenticated
	}
	if !o.validSession(current) {
		return domain.User{}, ErrUnauthenticated
	}

	user, err := o.repository.OIDCUser(r.Context(), current.Issuer, current.Subject)
	if errors.Is(err, domain.ErrNotFound) || (err == nil && !user.Enabled) {
		return domain.User{}, ErrUnauthenticated
	}
	if err != nil {
		return user, err
	}
	if current.Version != user.SessionVersion {
		return domain.User{}, ErrUnauthenticated
	}

	if o.adminGroup != "" && user.ExternalAdmin {
		user.Role = "admin"
	}

	return user, nil
}

// validSession reports whether a decoded browser session is current and bound to this provider.
func (o *OIDC) validSession(current session) bool {
	if current.Version <= 0 {
		return false
	}
	if current.Expires <= time.Now().Unix() {
		return false
	}
	if current.Issuer != o.issuer {
		return false
	}

	return current.Subject != ""
}

// login starts the OIDC authorization-code flow.
func (o *OIDC) login(w http.ResponseWriter, r *http.Request) {
	stateBytes := make([]byte, 24)
	if _, err := rand.Read(stateBytes); err != nil {
		httpresponse.Problem(w, http.StatusInternalServerError, "The request could not be processed.")
		return
	}

	state := base64.RawURLEncoding.EncodeToString(stateBytes)
	verifier := oauth2.GenerateVerifier()
	next := r.URL.Query().Get("next")
	if !httpresponse.IsLocalPath(next) {
		next = "/"
	}

	o.setCookie(w, oidcStateCookie, loginState{
		State:    state,
		Verifier: verifier,
		Next:     next,
		Expires:  time.Now().Add(oidcLoginStateTTL).Unix(),
	}, int(oidcLoginStateTTL.Seconds()))

	http.Redirect(
		w,
		r,
		o.oauth.AuthCodeURL(
			state,
			oauth2.S256ChallengeOption(verifier),
			oidc.Nonce(state),
		),
		http.StatusFound,
	)
}

// callback completes the OIDC flow and establishes the browser session.
func (o *OIDC) callback(w http.ResponseWriter, r *http.Request) {
	saved, ok := o.callbackLoginState(r)
	if !ok {
		httpresponse.Problem(w, http.StatusBadRequest, "Invalid login state.")
		return
	}

	// Consume transient browser state on both successful and failed callbacks.
	o.setCookie(w, oidcStateCookie, loginState{}, -1)
	// Clear the legacy redirect cookie left by pre-login-state deployments.
	o.setCookie(w, oidcLegacyNextCookie, struct{}{}, -1)

	token, err := o.oauth.Exchange(
		r.Context(),
		r.URL.Query().Get("code"),
		oauth2.VerifierOption(saved.Verifier),
	)
	if err != nil {
		httpresponse.Problem(w, http.StatusUnauthorized, "Login failed.")
		return
	}

	raw, _ := token.Extra("id_token").(string)

	idToken, err := o.verifier.Verify(r.Context(), raw)
	if err != nil {
		httpresponse.Problem(w, http.StatusUnauthorized, "Invalid identity.")
		return
	}

	issuer, subject, ok := o.callbackIdentity(idToken, saved.State)
	if !ok {
		httpresponse.Problem(w, http.StatusUnauthorized, "Invalid identity.")
		return
	}

	var identity claims
	if err := idToken.Claims(&identity); err != nil {
		httpresponse.Problem(w, http.StatusUnauthorized, "Invalid claims.")
		return
	}

	username := strings.TrimSpace(identity.PreferredUsername)
	if username == "" {
		httpresponse.Problem(w, http.StatusUnauthorized, "The preferred_username claim is required.")
		return
	}

	groups, err := o.callbackGroups(idToken)
	if err != nil {
		httpresponse.Problem(w, http.StatusUnauthorized, "Invalid group claim.")
		return
	}

	user, err := o.repository.LoginOIDCUser(
		r.Context(),
		issuer,
		subject,
		username,
		identity.Email,
		identity.Name,
	)
	if err != nil {
		writeOIDCLoginProblem(w, err)
		return
	}

	if !user.Enabled {
		httpresponse.Problem(w, http.StatusForbidden, "This account is disabled.")
		return
	}

	if err := o.syncAuthorization(r.Context(), user.ID, groups); err != nil {
		httpresponse.Problem(w, http.StatusInternalServerError, "The request could not be processed.")
		return
	}

	o.setCookie(w, oidcSessionCookie, session{
		Issuer:  issuer,
		Subject: subject,
		Expires: time.Now().Add(oidcSessionTTL).Unix(),
		Version: user.SessionVersion,
	}, int(oidcSessionTTL.Seconds()))

	next := "/"
	if httpresponse.IsLocalPath(saved.Next) {
		next = saved.Next
	}

	http.Redirect(w, r, next, http.StatusFound)
}

// callbackGroups extracts external groups only when authorization settings require them.
func (o *OIDC) callbackGroups(idToken *oidc.IDToken) ([]string, error) {
	if !o.groupSync && o.adminGroup == "" {
		return nil, nil
	}

	return oidcGroups(idToken, o.groupClaim)
}

// syncAuthorization synchronizes mapped groups and externally asserted administrator status.
func (o *OIDC) syncAuthorization(ctx context.Context, userID int64, groups []string) error {
	if o.groupSync {
		if err := o.repository.SyncOIDCGroups(ctx, userID, groups, o.groupMappings, o.groupsAuthoritative); err != nil {
			return err
		}
	}

	if o.adminGroup == "" {
		return nil
	}

	return o.repository.SetExternalAdminStatus(ctx, userID, "oidc", containsGroup(groups, o.adminGroup))
}

// writeOIDCLoginProblem maps identity-registration failures to stable browser responses.
func writeOIDCLoginProblem(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, domain.ErrIdentityApprovalRequired):
		httpresponse.Problem(
			w,
			http.StatusForbidden,
			"Registration is closed. Your verified identity is awaiting administrator approval.",
		)
	case errors.Is(err, domain.ErrIdentityRejected):
		httpresponse.Problem(w, http.StatusForbidden, "This identity has been rejected by an administrator.")
	case errors.Is(err, domain.ErrRegistrationDisabled):
		httpresponse.Problem(w, http.StatusForbidden, "User registration is disabled.")
	case errors.Is(err, domain.ErrAlreadyExists):
		httpresponse.Problem(w, http.StatusConflict, "The preferred username is already used by another account.")
	default:
		httpresponse.Problem(w, http.StatusInternalServerError, "The request could not be processed.")
	}
}

// callbackIdentity validates the issuer, subject, and nonce bound to an OIDC callback.
func (o *OIDC) callbackIdentity(idToken *oidc.IDToken, expectedNonce string) (issuer, subject string, valid bool) {
	issuer = strings.TrimSpace(idToken.Issuer)
	subject = strings.TrimSpace(idToken.Subject)

	if issuer == "" || subject == "" {
		return "", "", false
	}
	if issuer != o.issuer {
		return "", "", false
	}
	if idToken.Nonce != expectedNonce {
		return "", "", false
	}

	return issuer, subject, true
}

// callbackLoginState reads and validates the OIDC login state bound to a callback.
func (o *OIDC) callbackLoginState(r *http.Request) (state loginState, valid bool) {
	if err := o.decodeCookie(r, oidcStateCookie, &state); err != nil {
		return loginState{}, false
	}

	if state.State == "" || state.Verifier == "" {
		return loginState{}, false
	}

	if state.State != r.URL.Query().Get("state") {
		return loginState{}, false
	}

	if state.Expires <= time.Now().Unix() {
		return loginState{}, false
	}

	return state, true
}

// containsGroup reports whether the normalized external values contain the configured administrator group.
func containsGroup(groups []string, expected string) bool {
	expected = strings.TrimSpace(expected)
	if expected == "" {
		return false
	}

	return slices.ContainsFunc(groups, func(group string) bool {
		return strings.TrimSpace(group) == expected
	})
}

// oidcGroups extracts a configurable top-level string or string-array group claim.
func oidcGroups(idToken *oidc.IDToken, claim string) ([]string, error) {
	claim = strings.TrimSpace(claim)
	if claim == "" {
		return nil, nil
	}

	var values map[string]json.RawMessage
	if err := idToken.Claims(&values); err != nil {
		return nil, err
	}

	return oidcGroupValues(values[claim])
}

// oidcGroupValues normalizes one optional OIDC group claim.
func oidcGroupValues(raw json.RawMessage) ([]string, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return nil, nil
	}

	var groups []string
	if err := json.Unmarshal(raw, &groups); err != nil {
		var group string
		if stringErr := json.Unmarshal(raw, &group); stringErr != nil {
			return nil, errors.New("decode OIDC group claim: expected a string or string array")
		}

		groups = []string{group}
	}

	seen := make(map[string]bool, len(groups))
	normalized := groups[:0]

	for _, group := range groups {
		group = strings.TrimSpace(group)
		if group == "" || seen[group] {
			continue
		}

		seen[group] = true
		normalized = append(normalized, group)
	}

	return normalized, nil
}

// Logout clears OIDC and local browser sessions and redirects home.
func Logout(local *Local) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if local != nil {
			local.ClearSession(w, r)
		}

		http.SetCookie(w, &http.Cookie{
			Name:     "kumbuka_session",
			Value:    "",
			Path:     "/",
			MaxAge:   -1,
			HttpOnly: true,
			SameSite: http.SameSiteLaxMode,
		})

		http.Redirect(w, r, "/", http.StatusFound)
	}
}

// LoginUnavailable redirects to the home page when the configured auth mode has no login flow.
func LoginUnavailable() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/", http.StatusFound)
	}
}

// setCookie encodes, signs, and writes a short-lived application cookie.
func (o *OIDC) setCookie(w http.ResponseWriter, name string, value any, maxAge int) {
	data, _ := json.Marshal(value)
	payload := base64.RawURLEncoding.EncodeToString(data)

	mac := hmac.New(sha256.New, o.secret)
	_, _ = mac.Write([]byte(payload))

	signed := payload + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil))

	http.SetCookie(w, &http.Cookie{
		Name:     name,
		Value:    signed,
		Path:     "/",
		MaxAge:   maxAge,
		HttpOnly: true,
		Secure:   strings.HasPrefix(o.publicURL, "https://"),
		SameSite: http.SameSiteLaxMode,
	})
}

// decodeCookie verifies and decodes a signed application cookie.
func (o *OIDC) decodeCookie(r *http.Request, name string, out any) error {
	if len(o.secret) == 0 {
		return errors.New("no secret")
	}

	cookie, err := r.Cookie(name)
	if err != nil {
		return err
	}

	payload, signature, ok := strings.Cut(cookie.Value, ".")
	if !ok {
		return errors.New("invalid cookie")
	}

	got, err := base64.RawURLEncoding.DecodeString(signature)
	if err != nil {
		return err
	}

	mac := hmac.New(sha256.New, o.secret)
	_, _ = mac.Write([]byte(payload))

	if !hmac.Equal(got, mac.Sum(nil)) {
		return errors.New("invalid signature")
	}

	data, err := base64.RawURLEncoding.DecodeString(payload)
	if err != nil {
		return err
	}

	if err := json.Unmarshal(data, out); err != nil {
		return fmt.Errorf("decode cookie: %w", err)
	}

	return nil
}
