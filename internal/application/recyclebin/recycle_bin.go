package recyclebin

import (
	"context"

	"github.com/kumbuka-me/kumbuka/pkg/domain"
)

// recycleBinRepository contains deleted-page lifecycle operations.
type recycleBinRepository interface {
	DeletedPages(context.Context) ([]domain.DeletedPage, error)
	RestorePage(context.Context, string) error
	PermanentlyDeletePage(context.Context, string) error
}

// NavigationIconInvalidator invalidates navigation icons after permanent deletion removes orphaned paths.
type NavigationIconInvalidator interface {
	// InvalidateIcons discards any cached navigation icon snapshot.
	InvalidateIcons()
}

// RecycleBin exposes deleted-page lifecycle use cases.
type RecycleBin struct {
	// repository provides the persistence operations required by recycle bin.
	repository recycleBinRepository
	// navigationIcons invalidates cached paths after orphaned icons are removed.
	navigationIcons NavigationIconInvalidator
}

// NewRecycleBin constructs the deleted-page service.
func NewRecycleBin(repository recycleBinRepository, navigationIcons NavigationIconInvalidator) *RecycleBin {
	return &RecycleBin{repository: repository, navigationIcons: navigationIcons}
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
	if err := s.repository.PermanentlyDeletePage(ctx, slug); err != nil {
		return err
	}
	s.navigationIcons.InvalidateIcons()

	return nil
}
