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
	"github.com/kumbuka-me/kumbuka/pkg/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/oauth2"
)

// restartOIDCRepository provides test state for restart OIDC repository behavior.
type restartOIDCRepository struct {
	setupBrowserRepository
	oidcRepositoryStub
}

// restartOIDCFixture groups the state required by restart OIDC tests.
type restartOIDCFixture struct {
	// t holds the testing handle used by the fixture.
	t *testing.T
	// issuer configures or records the issuer value used by the fixture.
	issuer string
	// nonce configures or records the nonce value used by the fixture.
	nonce string
	// challenge configures or records the challenge value used by the fixture.
	challenge string
	// provider provides the provider dependency used by the fixture.
	provider *httptest.Server
	// repository provides the repository dependency used by the fixture.
	repository *restartOIDCRepository
	// config configures the config used by the fixture.
	config BrowserConfig
}

func newRestartOIDCFixture(t *testing.T) *restartOIDCFixture {
	t.Helper()

	key, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	signer, err := jose.NewSigner(jose.SigningKey{Algorithm: jose.RS256, Key: key}, nil)
	require.NoError(t, err)

	fixture := &restartOIDCFixture{t: t}
	fixture.provider = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fixture.serveProviderRequest(w, r, key, signer)
	}))
	fixture.issuer = fixture.provider.URL
	fixture.repository = &restartOIDCRepository{user: domain.User{ID: 7, Enabled: true, Role: "viewer", SessionVersion: 1}}
	fixture.repository.settings.Authentication = domain.AuthenticationSettings{Mode: "oidc", OIDCIssuer: fixture.issuer, OIDCClientID: "kumbuka"}
	fixture.config = BrowserConfig{OIDC: OIDCConfig{ClientSecret: "client-secret", SessionSecret: strings.Repeat("s", 32), PublicURL: "https://wiki.example"}}

	t.Cleanup(fixture.provider.Close)
	return fixture
}

func (f *restartOIDCFixture) serveProviderRequest(w http.ResponseWriter, r *http.Request, key *rsa.PrivateKey, signer jose.Signer) {
	f.t.Helper()
	w.Header().Set("Content-Type", "application/json")

	switch r.URL.Path {
	case "/.well-known/openid-configuration":
		_ = json.NewEncoder(w).Encode(map[string]any{"issuer": f.issuer, "authorization_endpoint": f.issuer + "/authorize", "token_endpoint": f.issuer + "/token", "jwks_uri": f.issuer + "/keys", "id_token_signing_alg_values_supported": []string{"RS256"}})
	case "/keys":
		_ = json.NewEncoder(w).Encode(jose.JSONWebKeySet{Keys: []jose.JSONWebKey{{Key: &key.PublicKey, Algorithm: "RS256", Use: "sig"}}})
	case "/token":
		f.serveTokenResponse(w, r, signer)
	default:
		http.NotFound(w, r)
	}
}

func (f *restartOIDCFixture) serveTokenResponse(w http.ResponseWriter, r *http.Request, signer jose.Signer) {
	f.t.Helper()
	if r.ParseForm() != nil || r.Form.Get("code") != "valid-code" || oauth2.S256ChallengeFromVerifier(r.Form.Get("code_verifier")) != f.challenge {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"invalid_grant"}`))
		return
	}

	claims, _ := json.Marshal(map[string]any{"iss": f.issuer, "sub": "subject-7", "aud": "kumbuka", "exp": time.Now().Add(time.Hour).Unix(), "iat": time.Now().Unix(), "nonce": f.nonce, "preferred_username": "alice"})
	signed, err := signer.Sign(claims)
	if !assert.NoError(f.t, err) {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	token, err := signed.CompactSerialize()
	if !assert.NoError(f.t, err) {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	_ = json.NewEncoder(w).Encode(map[string]any{"access_token": "access", "token_type": "Bearer", "id_token": token})
}

func (f *restartOIDCFixture) restart(t *testing.T) BrowserAuth {
	t.Helper()
	configured, err := ConfigureBrowserAuth(context.Background(), f.config, f.repository)
	require.NoError(t, err)
	return configured
}

func (f *restartOIDCFixture) beginLogin(t *testing.T) (*httptest.ResponseRecorder, *url.URL) {
	t.Helper()
	login := httptest.NewRecorder()
	f.restart(t).Login.ServeHTTP(login, httptest.NewRequest("GET", "/auth/login?next=%2Fpages%2Fstart", nil))
	require.Equal(t, http.StatusFound, login.Code)
	destination, err := url.Parse(login.Header().Get("Location"))
	require.NoError(t, err)
	f.nonce = destination.Query().Get("nonce")
	f.challenge = destination.Query().Get("code_challenge")
	require.NotEmpty(t, f.nonce)
	require.NotEmpty(t, f.challenge)
	require.Equal(t, "S256", destination.Query().Get("code_challenge_method"))
	return login, destination
}

func callbackRequest(login *httptest.ResponseRecorder, destination *url.URL) *http.Request {
	request := httptest.NewRequest("GET", "/auth/callback?code=valid-code&state="+url.QueryEscape(destination.Query().Get("state")), nil)
	for _, cookie := range login.Result().Cookies() {
		request.AddCookie(cookie)
	}
	return request
}

// Exercise real discovery, code exchange, JWKS signature verification and cookie handling across restart boundaries.
func TestOIDCLoginAcrossRestarts(t *testing.T) {
	fixture := newRestartOIDCFixture(t)

	t.Run("survives restarts", func(t *testing.T) {
		login, destination := fixture.beginLogin(t)
		response := httptest.NewRecorder()
		fixture.restart(t).Callback.ServeHTTP(response, callbackRequest(login, destination))
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

		third := fixture.restart(t)
		user, err := third.Authenticator.Authenticate(request)
		require.NoError(t, err)
		assert.Equal(t, int64(7), user.ID)

		fixture.repository.user.SessionVersion++
		_, err = third.Authenticator.Authenticate(request)
		require.ErrorIs(t, err, ErrUnauthenticated)
		fixture.repository.user.SessionVersion--

		fixture.config.OIDC.SessionSecret = strings.Repeat("t", 32)
		_, err = fixture.restart(t).Authenticator.Authenticate(request)
		require.ErrorIs(t, err, ErrUnauthenticated)
		fixture.config.OIDC.SessionSecret = strings.Repeat("s", 32)
	})

	t.Run("rejects unrelated identity token", func(t *testing.T) {
		login, destination := fixture.beginLogin(t)
		fixture.nonce = "unrelated-login"
		response := httptest.NewRecorder()
		fixture.restart(t).Callback.ServeHTTP(response, callbackRequest(login, destination))
		assert.Equal(t, http.StatusUnauthorized, response.Code)
	})
}
