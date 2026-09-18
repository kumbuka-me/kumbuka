package externalfiles

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"testing"

	"github.com/kumbuka-me/kumbuka/internal/auth"
	"github.com/kumbuka-me/kumbuka/internal/secrets"
	"github.com/kumbuka-me/kumbuka/pkg/domain"
	sdk "github.com/kumbuka-me/sdk"
)

type memoryStore map[string][]byte

func (m memoryStore) ReadPluginValue(_ context.Context, id, ns, k string) ([]byte, bool, error) {
	v, ok := m[id+"/"+ns+"/"+k]
	return v, ok, nil
}
func (m memoryStore) ListPluginValues(_ context.Context, id, ns, p string) (map[string][]byte, error) {
	out := map[string][]byte{}
	for k, v := range m {
		if strings.HasPrefix(k, id+"/"+ns+"/"+p) {
			out[k] = v
		}
	}
	return out, nil
}
func (m memoryStore) WritePluginValue(_ context.Context, id, ns, k string, v []byte) error {
	m[id+"/"+ns+"/"+k] = v
	return nil
}
func (m memoryStore) DeletePluginValue(_ context.Context, id, ns, k string) error {
	delete(m, id+"/"+ns+"/"+k)
	return nil
}
func source() Source {
	return Source{ID: "docs", Provider: "github", Endpoint: "https://api.github.com", Repository: "team/docs", Ref: "main", Enabled: true}
}
func testService(t *testing.T) (*Service, memoryStore) {
	t.Helper()
	cipher, err := secrets.New(base64.StdEncoding.EncodeToString(make([]byte, 32)))
	if err != nil {
		t.Fatal(err)
	}
	store := memoryStore{}
	return New(store, cipher), store
}

func TestApprovalCredentialAndRevocation(t *testing.T) {
	s, store := testService(t)
	ctx := context.Background()
	if err := s.Save(ctx, source(), "secret-token"); err != nil {
		t.Fatal(err)
	}
	for _, data := range store {
		if strings.Contains(string(data), "secret-token") {
			t.Fatal("plaintext stored")
		}
	}
	rows, err := s.List(ctx)
	if err != nil || len(rows) != 1 || rows[0].Token != "" || !rows[0].HasToken {
		t.Fatalf("metadata: %+v %v", rows, err)
	}
	calls := 0
	s.fetch = func(_ context.Context, v Source, p string) (string, error) {
		calls++
		if v.Token != "secret-token" || p != "file.txt" {
			t.Fatal("wrong credential binding")
		}
		return "first\nsecond\n", nil
	}
	q := sdk.ExternalFileRequest{Source: "docs", Path: "file.txt", Start: 2, End: 2}
	raw, _ := json.Marshal(q)
	if _, err := s.Capability(ctx, raw); err == nil || calls != 0 {
		t.Fatal("anonymous read allowed")
	}
	request := auth.WithUser(httptest.NewRequest("GET", "/", nil), domain.User{ID: 1})
	result, err := s.Capability(request.Context(), raw)
	if err != nil || result.(sdk.ExternalFile).Content != "second" {
		t.Fatalf("read %v %v", result, err)
	}
	if _, err = s.Capability(request.Context(), json.RawMessage(`{"source":"docs","path":"file.txt","token":"override"}`)); err == nil {
		t.Fatal("unknown request field accepted")
	}
	if err = s.Delete(ctx, "docs"); err != nil {
		t.Fatal(err)
	}
	if _, err = s.read(ctx, q); err == nil {
		t.Fatal("revoked source readable")
	}
	if calls != 1 {
		t.Fatalf("calls=%d", calls)
	}
}
func TestReplacementDoesNotReuseCredential(t *testing.T) {
	s, _ := testService(t)
	ctx := context.Background()
	v := source()
	if err := s.Save(ctx, v, "secret"); err != nil {
		t.Fatal(err)
	}
	v.Endpoint = "https://other.example/api/v3"
	if err := s.Save(ctx, v, ""); err != nil {
		t.Fatal(err)
	}
	s.fetch = func(_ context.Context, v Source, _ string) (string, error) {
		if v.Token != "" {
			t.Fatal("credential reused")
		}
		return "ok", nil
	}
	if _, err := s.read(ctx, sdk.ExternalFileRequest{Source: "docs", Path: "x"}); err != nil {
		t.Fatal(err)
	}
}
func TestInvalidSourcesAndPaths(t *testing.T) {
	for _, endpoint := range []string{"http://api.github.com", "https://u:p@api.github.com", "https://api.github.com?token=x", "https://api.github.com/#x", "https://api.github.com/%2e%2e", "https://api.github.com/a/../b"} {
		v := source()
		v.Endpoint = endpoint
		if validateSource(v) == nil {
			t.Errorf("accepted %s", endpoint)
		}
	}
	for _, p := range []string{"", "../secret", "a/../b", "/file", "a//b", "a\\b", "%2e%2e/file", "x?ref=other", "x\x00"} {
		if validPath(p) {
			t.Errorf("accepted path %q", p)
		}
	}
	s, _ := testService(t)
	s.cipher = nil
	if s.Save(context.Background(), source(), "secret") == nil {
		t.Fatal("unencrypted credential accepted")
	}
}
func TestNetworkPolicy(t *testing.T) {
	for _, raw := range []string{"127.0.0.1", "0.0.0.0", "169.254.169.254", "10.0.0.1", "172.16.0.1", "192.168.1.1", "100.100.100.200", "192.0.2.1", "198.18.0.1", "224.0.0.1", "::1", "fe80::1", "fc00::1", "64:ff9b::7f00:1", "2002:7f00:1::", "2001:db8::1"} {
		if allowedIP(netip.MustParseAddr(raw), nil) {
			t.Errorf("allowed %s", raw)
		}
	}
	if !allowedIP(netip.MustParseAddr("10.0.0.1"), []string{"10.0.0.1"}) || allowedIP(netip.MustParseAddr("10.0.0.2"), []string{"10.0.0.1"}) {
		t.Fatal("private IP exception not exact")
	}
	for _, raw := range []string{"127.0.0.1", "169.254.169.254", "10.0.0.0/8", "::ffff:10.0.0.1"} {
		if validPrivateIP(raw) {
			t.Errorf("invalid private exception %s", raw)
		}
	}
	c, err := secureClient(source(), false)
	if err != nil {
		t.Fatal(err)
	}
	if c.Transport.(*http.Transport).Proxy != nil {
		t.Fatal("ambient proxy enabled")
	}
	if c.CheckRedirect(nil, nil) != http.ErrUseLastResponse {
		t.Fatal("redirect allowed")
	}
	if _, err := c.Transport.(*http.Transport).DialContext(context.Background(), "tcp", "127.0.0.1:443"); err == nil {
		t.Fatal("loopback dial allowed")
	}
}

type roundTrip func(*http.Request) (*http.Response, error)

func (f roundTrip) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func TestProviderRequestsAndBounds(t *testing.T) {
	for _, provider := range []string{"github", "gitlab"} {
		t.Run(provider, func(t *testing.T) {
			v := source()
			v.Provider = provider
			v.Token = "secret"
			v.Ref = "feature/a"
			if provider == "gitlab" {
				v.Endpoint = "https://git.example/api/v4"
				v.Repository = "group/sub/repo"
			}
			client := &http.Client{Transport: roundTrip(func(r *http.Request) (*http.Response, error) {
				if r.URL.Query().Get("ref") != "feature/a" || strings.Contains(r.URL.String(), "secret") {
					t.Fatal("bad request URL")
				}
				if provider == "github" && r.Header.Get("Authorization") != "Bearer secret" {
					t.Fatal("missing Github auth")
				}
				if provider == "gitlab" && r.Header.Get("PRIVATE-TOKEN") != "secret" {
					t.Fatal("missing Gitlab auth")
				}
				body := "hello\n"
				if provider == "github" {
					body = `{"type":"file","encoding":"base64","size":6,"content":"aGVsbG8K"}`
				}
				return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))}, nil
			})}
			got, err := fetchWithClient(context.Background(), client, v, "src/file name.go")
			if err != nil || got != "hello\n" {
				t.Fatalf("%q %v", got, err)
			}
		})
	}
	for _, body := range []string{`{"type":"dir"}`, `{"type":"symlink"}`, `{"type":"file","encoding":"none"}`, strings.Repeat("a", 2*maxFile+1)} {
		client := &http.Client{Transport: roundTrip(func(*http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))}, nil
		})}
		if _, err := fetchWithClient(context.Background(), client, source(), "x"); err == nil {
			t.Fatal("bad provider body accepted")
		}
	}
}
func TestTextAndRanges(t *testing.T) {
	for _, content := range []string{"\xff", "\x00", "x\u202ey", strings.Repeat("a", maxFile+1)} {
		if validContent(content) {
			t.Fatal("invalid content accepted")
		}
	}
	got, err := selectLines("a\r\nb\r\n", sdk.ExternalFileRequest{})
	if err != nil || got.Start != 1 || got.Content != "a\nb" {
		t.Fatalf("%+v %v", got, err)
	}
	if _, err := selectLines("a\n", sdk.ExternalFileRequest{Start: 2, End: 2}); err == nil {
		t.Fatal("phantom trailing line")
	}
	s, _ := testService(t)
	if err := s.Save(context.Background(), source(), ""); err != nil {
		t.Fatal(err)
	}
	s.fetch = func(context.Context, Source, string) (string, error) {
		return "", errors.New("token secret provider body")
	}
	_, err = s.read(context.Background(), sdk.ExternalFileRequest{Source: "docs", Path: "x"})
	if err == nil || strings.Contains(err.Error(), "secret") {
		t.Fatal("upstream error leaked")
	}
}

func TestRateAndConcurrencyLimits(t *testing.T) {
	s, _ := testService(t)
	ctx := context.Background()
	if err := s.Save(ctx, source(), ""); err != nil {
		t.Fatal(err)
	}
	calls := 0
	s.fetch = func(context.Context, Source, string) (string, error) { calls++; return "ok", nil }
	q := sdk.ExternalFileRequest{Source: "docs", Path: "x"}
	for i := 0; i < 30; i++ {
		if _, err := s.read(ctx, q); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := s.read(ctx, q); err == nil || calls != 30 {
		t.Fatal("rate limit not applied")
	}
	s.rates = make(map[string]rate)
	for i := 0; i < cap(s.active); i++ {
		s.active <- struct{}{}
	}
	if _, err := s.read(ctx, q); err == nil || calls != 30 {
		t.Fatal("concurrency limit not applied")
	}
}

func TestRedirectAndErrorBodiesAreNotRead(t *testing.T) {
	for _, status := range []int{301, 302, 307, 401, 404, 429, 500} {
		client := &http.Client{Transport: roundTrip(func(*http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: status, Header: http.Header{"Location": []string{"https://attacker.example"}}, Body: io.NopCloser(strings.NewReader("private upstream details"))}, nil
		}), CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
		_, err := fetchWithClient(context.Background(), client, source(), "x")
		if err == nil || strings.Contains(err.Error(), "private") {
			t.Fatal("unsafe response", status, err)
		}
	}
}
