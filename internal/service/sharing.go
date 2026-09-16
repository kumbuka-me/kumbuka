package service

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"strings"

	"github.com/kumbuka-me/kumbuka/pkg/domain"
)

const pageShareTokenBytes = 32

// IssuedPageShareLink contains the raw public token returned only when a permalink is created.
type IssuedPageShareLink struct {
	Token string
}

// sharingRepository contains public page sharing operations.
type sharingRepository interface {
	auditRepository
	GetPage(context.Context, string) (domain.Page, error)
	CreatePageShareLink(context.Context, int64, int64, string) error
	PageShareLink(context.Context, string) (domain.PageShareLink, error)
}

// Sharing coordinates public page permalink use cases.
type Sharing struct {
	repository sharingRepository
}

// NewSharing constructs the public page sharing service.
func NewSharing(repository sharingRepository) *Sharing {
	return &Sharing{repository: repository}
}

// CreatePageShareLink creates an opaque public permalink for one page.
func (s *Sharing) CreatePageShareLink(
	ctx context.Context,
	slug string,
	actor domain.User,
) (IssuedPageShareLink, error) {
	page, err := s.repository.GetPage(ctx, strings.TrimSpace(slug))
	if err != nil {
		return IssuedPageShareLink{}, err
	}

	token, err := newPageShareToken()
	if err != nil {
		return IssuedPageShareLink{}, err
	}
	if err := s.repository.CreatePageShareLink(
		ctx,
		page.ID,
		actor.ID,
		pageShareTokenHash(token),
	); err != nil {
		return IssuedPageShareLink{}, err
	}

	_ = s.repository.LogAudit(
		ctx,
		actor.ID,
		"page.share_created",
		"page",
		page.Slug,
		"Created public permalink",
	)

	return IssuedPageShareLink{Token: token}, nil
}

// PageShareLink resolves an active public permalink.
func (s *Sharing) PageShareLink(ctx context.Context, token string) (domain.PageShareLink, error) {
	if !validPageShareToken(token) {
		return domain.PageShareLink{}, domain.ErrNotFound
	}
	return s.repository.PageShareLink(ctx, pageShareTokenHash(token))
}

// newPageShareToken creates a high-entropy URL-safe bearer token.
func newPageShareToken() (string, error) {
	data := make([]byte, pageShareTokenBytes)
	if _, err := rand.Read(data); err != nil {
		return "", fmt.Errorf("generate page share token: %w", err)
	}

	return base64.RawURLEncoding.EncodeToString(data), nil
}

// pageShareTokenHash returns the at-rest representation of a public share token.
func pageShareTokenHash(token string) string {
	hash := sha256.Sum256([]byte(token))
	return hex.EncodeToString(hash[:])
}

// validPageShareToken rejects malformed tokens before hitting PostgreSQL.
func validPageShareToken(token string) bool {
	decoded, err := base64.RawURLEncoding.DecodeString(strings.TrimSpace(token))
	return err == nil && len(decoded) == pageShareTokenBytes
}
