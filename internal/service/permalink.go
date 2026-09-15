package service

import "context"

// PageSlugByID resolves the current page path for a stable page identifier.
func (s *Catalog) PageSlugByID(ctx context.Context, id int64) (string, error) {
	return s.repository.PageSlugByID(ctx, id)
}
