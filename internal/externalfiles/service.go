// Package externalfiles implements host-owned approval and repository fetching.
package externalfiles

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/url"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/kumbuka-me/kumbuka/internal/auth"
	"github.com/kumbuka-me/kumbuka/internal/secrets"
	"github.com/kumbuka-me/kumbuka/pkg/plugin"
	sdk "github.com/kumbuka-me/sdk"
)

const owner = "core.external-files"
const namespace = "approved-sources"
const maxFile = 128 << 10

var identifier = regexp.MustCompile(`^[a-z][a-z0-9-]{0,63}$`)
var errUnavailable = errors.New("external file unavailable; check source approval, access, limits and provider status")

// Source is an administrator's explicit approval to disclose a repository's
// files at Ref to Kumbuka users. Token is encrypted and never sent to guests.
type Source struct {
	ID         string
	Provider   string
	Endpoint   string
	Repository string
	Ref        string
	// PrivateIPs permits exact RFC1918/ULA addresses for internal Git servers.
	PrivateIPs []string
	Enabled    bool
	Token      string `json:"token,omitempty"`
	HasToken   bool   `json:"-"`
}

// Service owns credential access; plugin storage permissions cannot address its namespace.
type Service struct {
	insecureTLS bool
	storage     plugin.Storage
	cipher      *secrets.Cipher
	mu          sync.Mutex
	rates       map[string]rate
	active      chan struct{}
	fetch       func(context.Context, Source, string) (string, error)
}
type rate struct {
	since time.Time
	count int
}

// New creates an external-file service. Missing encryption configuration prevents token saves.
func New(storage plugin.Storage, cipher *secrets.Cipher, options ...Option) *Service {
	s := &Service{storage: storage, cipher: cipher, rates: make(map[string]rate), active: make(chan struct{}, 8)}
	for _, option := range options {
		option(s)
	}
	s.fetch = func(ctx context.Context, v Source, path string) (string, error) {
		return fetchFile(ctx, v, path, s.insecureTLS)
	}
	return s
}

// Option configures deployment-controlled networking policy.
type Option func(*Service)

// WithInsecureTLS disables provider certificate verification only when explicitly enabled.
func WithInsecureTLS(enabled bool) Option { return func(s *Service) { s.insecureTLS = enabled } }

// List returns metadata only, including whether a credential is configured.
func (s *Service) List(ctx context.Context) ([]Source, error) {
	rows, err := s.storage.ListPluginValues(ctx, owner, namespace, "")
	if err != nil {
		return nil, err
	}
	out := make([]Source, 0, len(rows))
	for _, data := range rows {
		var v Source
		if json.Unmarshal(data, &v) != nil {
			return nil, errUnavailable
		}
		v.HasToken = v.Token != ""
		v.Token = ""
		out = append(out, v)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

// Save replaces an approval. Credentials must be supplied again on replacement,
// so changing the destination can never forward an existing token to a new host.
func (s *Service) Save(ctx context.Context, v Source, token string) error {
	v.Token = ""
	v.HasToken = false
	if err := validateSource(v); err != nil {
		return err
	}
	if len(token) > 4096 || strings.IndexFunc(token, unicode.IsControl) >= 0 {
		return errors.New("invalid provider token")
	}
	if token != "" {
		var err error
		v.Token, err = s.cipher.Encrypt(token)
		if err != nil {
			return errors.New("configure the application encryption key before saving credentials")
		}
	}
	data, err := json.Marshal(v)
	if err != nil {
		return err
	}
	return s.storage.WritePluginValue(ctx, owner, namespace, v.ID, data)
}

// Delete immediately removes an approval for subsequent requests.
func (s *Service) Delete(ctx context.Context, id string) error {
	if !identifier.MatchString(id) {
		return errors.New("invalid source name")
	}
	return s.storage.DeletePluginValue(ctx, owner, namespace, id)
}

// Capability validates the authenticated host context before accessing credentials.
func (s *Service) Capability(ctx context.Context, raw json.RawMessage) (any, error) {
	user, ok := auth.ContextUser(ctx)
	if !ok || user.ID <= 0 {
		return nil, errUnavailable
	}
	var request sdk.ExternalFileRequest
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if d.Decode(&request) != nil {
		return nil, errUnavailable
	}
	var extra any
	if d.Decode(&extra) != io.EOF {
		return nil, errUnavailable
	}
	return s.read(ctx, request)
}

func (s *Service) read(ctx context.Context, q sdk.ExternalFileRequest) (sdk.ExternalFile, error) {
	fail := func() (sdk.ExternalFile, error) { return sdk.ExternalFile{}, errUnavailable }
	if !identifier.MatchString(q.Source) || !validPath(q.Path) || q.Start < 0 || q.End < 0 || q.End > 10000 || ((q.Start == 0) != (q.End == 0)) || q.Start > q.End {
		return fail()
	}
	data, found, err := s.storage.ReadPluginValue(ctx, owner, namespace, q.Source)
	if err != nil || !found {
		return fail()
	}
	var v Source
	if json.Unmarshal(data, &v) != nil || v.ID != q.Source || !v.Enabled || validateSource(v) != nil {
		return fail()
	}
	s.mu.Lock()
	now := time.Now()
	if len(s.rates) >= 1024 {
		for k, v := range s.rates {
			if now.Sub(v.since) >= time.Minute {
				delete(s.rates, k)
			}
		}
	}
	if _, exists := s.rates[q.Source]; !exists && len(s.rates) >= 1024 {
		s.mu.Unlock()
		return fail()
	}
	r := s.rates[q.Source]
	if now.Sub(r.since) >= time.Minute {
		r = rate{since: now}
	}
	if r.count >= 30 {
		s.mu.Unlock()
		return fail()
	}
	r.count++
	s.rates[q.Source] = r
	s.mu.Unlock()
	select {
	case s.active <- struct{}{}:
		defer func() { <-s.active }()
	default:
		return fail()
	}
	if v.Token != "" {
		v.Token, err = s.cipher.Decrypt(v.Token)
		if err != nil {
			return fail()
		}
	}
	ctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	content, err := s.fetch(ctx, v, q.Path)
	if err != nil {
		return fail()
	}
	return selectLines(content, q)
}

func validateSource(v Source) error {
	if !identifier.MatchString(v.ID) || (v.Provider != "github" && v.Provider != "gitlab") {
		return errors.New("use a source name and GitHub or GitLab provider")
	}
	u, err := url.Parse(v.Endpoint)
	if err != nil || u.Scheme != "https" || u.Hostname() == "" || u.User != nil || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || u.RawPath != "" || strings.ContainsAny(u.Host, "%\\") || len(v.Endpoint) > 512 {
		return errors.New("provider API endpoint must be an HTTPS URL without credentials, query or fragment")
	}
	// API prefixes are explicit; GitHub Enterprise usually uses /api/v3.
	if u.Path != "" && (!validPath(strings.Trim(u.Path, "/")) || strings.Contains(u.Path, "//")) {
		return errors.New("invalid API endpoint path")
	}
	if !validPath(v.Repository) || len(v.Repository) > 256 || (v.Provider == "github" && len(strings.Split(v.Repository, "/")) != 2) {
		return errors.New("invalid repository path")
	}
	if v.Ref == "" || len(v.Ref) > 256 || strings.IndexFunc(v.Ref, unicode.IsControl) >= 0 {
		return errors.New("an explicit branch, tag or commit is required")
	}
	if len(v.PrivateIPs) > 16 {
		return errors.New("too many private addresses")
	}
	for _, raw := range v.PrivateIPs {
		if !validPrivateIP(raw) {
			return errors.New("private address exceptions must be exact RFC1918 or IPv6 ULA addresses")
		}
	}
	return nil
}

func validPath(p string) bool {
	if p == "" || len(p) > 1024 || strings.ContainsAny(p, "\\%?#") || strings.IndexFunc(p, unicode.IsControl) >= 0 {
		return false
	}
	for _, part := range strings.Split(p, "/") {
		if part == "" || part == "." || part == ".." {
			return false
		}
	}
	return true
}

// InsecureTLS reports the deployment setting for the administrator warning.
func (s *Service) InsecureTLS() bool { return s.insecureTLS }
