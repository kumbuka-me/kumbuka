package service

import (
	"context"

	"github.com/kumbuka-me/kumbuka/internal/domain"
)

// recycleBinRepository contains deleted-page lifecycle operations.
type recycleBinRepository interface {
	DeletedPages(context.Context) ([]domain.DeletedPage, error)
	RestorePage(context.Context, string) error
	PermanentlyDeletePage(context.Context, string) error
}

// RecycleBin exposes deleted-page lifecycle use cases.
type RecycleBin struct{ repository recycleBinRepository }

// NewRecycleBin constructs the deleted-page service.
func NewRecycleBin(repository recycleBinRepository) *RecycleBin {
	return &RecycleBin{repository: repository}
}

// DeletedPages returns pages currently held in the recycle bin.
func (s *RecycleBin) DeletedPages(ctx context.Context) ([]domain.DeletedPage, error) {
	return s.repository.DeletedPages(ctx)
}

// RestorePage restores a page from the recycle bin.
func (s *RecycleBin) RestorePage(ctx context.Context, slug string) error {
	return s.repository.RestorePage(ctx, slug)
}

// PermanentlyDeletePage removes a page already held in the recycle bin.
func (s *RecycleBin) PermanentlyDeletePage(ctx context.Context, slug string) error {
	return s.repository.PermanentlyDeletePage(ctx, slug)
}
