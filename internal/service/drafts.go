package service

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/kumbuka-me/kumbuka/pkg/domain"
)

// pageDraftRepository persists private editor drafts independently from page revisions.
type pageDraftRepository interface {
	PageDraft(context.Context, int64, string) (domain.PageDraft, error)
	PageDrafts(context.Context, int64, int) ([]domain.PageDraft, error)
	SavePageDraft(context.Context, int64, string, int64, string, string, map[string][]string) (domain.PageDraft, error)
	DeletePageDraft(context.Context, int64, string) error
}

// PageDraftSaveInput contains one user's autosaved editor state.
type PageDraftSaveInput struct {
	Key    string
	PageID int64
	Title  string
	Slug   string
	Values map[string][]string
	Actor  domain.User
}

// Drafts coordinates private server-side editor drafts.
type Drafts struct {
	repository pageDraftRepository
}

// NewDrafts constructs the private page-draft service.
func NewDrafts(repository pageDraftRepository) *Drafts {
	return &Drafts{repository: repository}
}

// Draft returns one private draft owned by the supplied user.
func (s *Drafts) Draft(ctx context.Context, userID int64, key string) (domain.PageDraft, error) {
	if userID <= 0 || !validPageDraftKey(key, 0, false) {
		return domain.PageDraft{}, domain.ErrNotFound
	}
	return s.repository.PageDraft(ctx, userID, strings.TrimSpace(key))
}

// List returns a user's most recently updated private drafts.
func (s *Drafts) List(ctx context.Context, userID int64, limit int) ([]domain.PageDraft, error) {
	if userID <= 0 || limit <= 0 {
		return nil, nil
	}
	return s.repository.PageDrafts(ctx, userID, limit)
}

// Save validates and persists one private editor draft without creating a page revision.
func (s *Drafts) Save(ctx context.Context, input PageDraftSaveInput) (domain.PageDraft, error) {
	input.Key = strings.TrimSpace(input.Key)
	if input.Actor.ID <= 0 {
		return domain.PageDraft{}, domain.ErrForbidden
	}
	if input.PageID < 0 || !validPageDraftKey(input.Key, input.PageID, true) {
		return domain.PageDraft{}, newValidationError("draft", "Invalid page draft identifier.")
	}

	if input.Values == nil {
		input.Values = map[string][]string{}
	}

	return s.repository.SavePageDraft(
		ctx,
		input.Actor.ID,
		input.Key,
		input.PageID,
		strings.TrimSpace(input.Title),
		strings.TrimSpace(input.Slug),
		input.Values,
	)
}

// Delete discards one private editor draft owned by the supplied user.
func (s *Drafts) Delete(ctx context.Context, userID int64, key string) error {
	key = strings.TrimSpace(key)
	if userID <= 0 || !validPageDraftKey(key, 0, false) {
		return nil
	}

	return s.repository.DeletePageDraft(ctx, userID, key)
}

// validPageDraftKey validates the stable editor key used by browser and server drafts.
func validPageDraftKey(key string, pageID int64, requireMatch bool) bool {
	key = strings.TrimSpace(key)
	if key == "new" {
		return !requireMatch || pageID == 0
	}

	const prefix = "page:"
	idText, ok := strings.CutPrefix(key, prefix)
	if !ok {
		return false
	}

	id, err := strconv.ParseInt(idText, 10, 64)
	if err != nil || id <= 0 {
		return false
	}

	return !requireMatch || id == pageID
}

// PageDraftKey returns the stable private-draft key for a persisted page.
func PageDraftKey(pageID int64) string {
	if pageID <= 0 {
		return "new"
	}
	return fmt.Sprintf("page:%d", pageID)
}
