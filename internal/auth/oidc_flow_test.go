package auth

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/go-jose/go-jose/v4"
	"github.com/kumbuka-me/kumbuka/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/oauth2"
)

type restartOIDCRepository struct {
	setupBrowserRepository
	oidcRepositoryStub
}

// Exercise real discovery, code exchange, JWKS signature verification and cookie
// handling, reconstructing browser authentication at each restart boundary.
func TestOIDCLoginAcrossRestarts(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	signer, err := jose.NewSigner(jose.SigningKey{Algorithm: jose.RS256, Key: key}, nil)
	require.NoError(t, err)
	var issuer, nonce, challenge string
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/.well-known/openid-configuration":
			_ = json.NewEncoder(w).Encode(map[string]any{"issuer": issuer, "authorization_endpoint": issuer + "/authorize", "token_endpoint": issuer + "/token", "jwks_uri": issuer + "/keys", "id_token_signing_alg_values_supported": []string{"RS256"}})
		case "/keys":
			_ = json.NewEncoder(w).Encode(jose.JSONWebKeySet{Keys: []jose.JSONWebKey{{Key: &key.PublicKey, Algorithm: "RS256", Use: "sig"}}})
		case "/token":
			if r.ParseForm() != nil || r.Form.Get("code") != "valid-code" || oauth2.S256ChallengeFromVerifier(r.Form.Get("code_verifier")) != challenge {
				w.WriteHeader(http.StatusBadRequest)
				_, _ = w.Write([]byte(`{"error":"invalid_grant"}`))
				return
			}
			claims, _ := json.Marshal(map[string]any{"iss": issuer, "sub": "subject-7", "aud": "kumbuka", "exp": time.Now().Add(time.Hour).Unix(), "iat": time.Now().Unix(), "nonce": nonce, "preferred_username": "alice"})
			signed, signErr := signer.Sign(claims)
			if !assert.NoError(t, signErr) {
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
			token, serializeErr := signed.CompactSerialize()
			if !assert.NoError(t, serializeErr) {
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"access_token": "access", "token_type": "Bearer", "id_token": token})
		default:
			http.NotFound(w, r)
		}
	}))
	defer provider.Close()
	issuer = provider.URL
	repository := &restartOIDCRepository{
		user: domain.User{
			ID:             7,
			Enabled:        true,
			Role:           "viewer",
			SessionVersion: 1,
		},
	}
	repository.settings.Authentication = domain.AuthenticationSettings{Mode: "oidc", OIDCIssuer: issuer, OIDCClientID: "kumbuka"}
	config := BrowserConfig{OIDC: OIDCConfig{ClientSecret: "client-secret", SessionSecret: strings.Repeat("s", 32), PublicURL: "https://wiki.example"}}
	restart := func(t *testing.T) BrowserAuth {
		t.Helper()
		configured, err := ConfigureBrowserAuth(context.Background(), config, repository)
		require.NoError(t, err)
		return configured
	}
	t.Run("survives restarts", func(t *testing.T) {
		first := restart(t)
		login := httptest.NewRecorder()
		first.Login.ServeHTTP(login, httptest.NewRequest("GET", "/auth/login?next=%2Fpages%2Fstart", nil))
		require.Equal(t, http.StatusFound, login.Code)
		destination, err := url.Parse(login.Header().Get("Location"))
		require.NoError(t, err)
		nonce = destination.Query().Get("nonce")
		challenge = destination.Query().Get("code_challenge")
		require.NotEmpty(t, nonce)
		require.NotEmpty(t, challenge)
		require.Equal(t, "S256", destination.Query().Get("code_challenge_method"))
		callback := httptest.NewRequest("GET", "/auth/callback?code=valid-code&state="+url.QueryEscape(destination.Query().Get("state")), nil)
		for _, cookie := range login.Result().Cookies() {
			callback.AddCookie(cookie)
		}
		second := restart(t)
		response := httptest.NewRecorder()
		second.Callback.ServeHTTP(response, callback)
		require.Equal(t, http.StatusFound, response.Code, response.Body.String())
		require.Equal(t, "/pages/start", response.Header().Get("Location"))
		request := httptest.NewRequest("GET", "/pages/start", nil)
		for _, cookie := range response.Result().Cookies() {
			if cookie.Name == "kumbuka_session" {
				request.AddCookie(cookie)
			} else {
				assert.Equal(t, -1, cookie.MaxAge)
			}
		}
		third := restart(t)
		user, err := third.Authenticator.Authenticate(request)
		require.NoError(t, err)
		assert.Equal(t, int64(7), user.ID)
		repository.user.SessionVersion++
		_, err = third.Authenticator.Authenticate(request)
		require.ErrorIs(t, err, ErrUnauthenticated)
		repository.user.SessionVersion--
		config.OIDC.SessionSecret = strings.Repeat("t", 32)
		fourth := restart(t)
		_, err = fourth.Authenticator.Authenticate(request)
		require.ErrorIs(t, err, ErrUnauthenticated)
		config.OIDC.SessionSecret = strings.Repeat("s", 32)
	})

	t.Run("rejects unrelated identity token", func(t *testing.T) {
		first := restart(t)
		login := httptest.NewRecorder()
		first.Login.ServeHTTP(login, httptest.NewRequest("GET", "/auth/login?next=%2Fpages%2Fstart", nil))
		require.Equal(t, http.StatusFound, login.Code)
		destination, err := url.Parse(login.Header().Get("Location"))
		require.NoError(t, err)
		nonce = destination.Query().Get("nonce")
		challenge = destination.Query().Get("code_challenge")
		require.NotEmpty(t, nonce)
		require.NotEmpty(t, challenge)
		require.Equal(t, "S256", destination.Query().Get("code_challenge_method"))
		nonce = "unrelated-login"
		callback := httptest.NewRequest("GET", "/auth/callback?code=valid-code&state="+url.QueryEscape(destination.Query().Get("state")), nil)
		for _, cookie := range login.Result().Cookies() {
			callback.AddCookie(cookie)
		}
		second := restart(t)
		response := httptest.NewRecorder()
		second.Callback.ServeHTTP(response, callback)
		assert.Equal(t, http.StatusUnauthorized, response.Code)
	})
}
