package plugincap

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/kumbuka-me/kumbuka/pkg/domain"
	"github.com/kumbuka-me/kumbuka/pkg/plugin"
	"github.com/kumbuka-me/sdk"
)

// RecentPagesSource exposes the newest authorized pages.
type RecentPagesSource interface {
	Recent(context.Context, int) ([]domain.Page, error)
}

// RecentViewedSource exposes pages recently viewed by the current viewer.
type RecentViewedSource interface {
	RecentViewed(context.Context, int) ([]domain.Page, error)
}

// FavoritePagesSource exposes pages favorited by the current viewer.
type FavoritePagesSource interface {
	Favorites(context.Context, int) ([]domain.Page, error)
}

// PopularPagesSource exposes the most-viewed authorized pages.
type PopularPagesSource interface {
	Popular(context.Context, int) ([]domain.Page, error)
}

// RecentEditsSource exposes pages recently edited by the current viewer.
type RecentEditsSource interface {
	RecentEdited(context.Context, int) ([]domain.RecentEdit, error)
}

// DraftSource exposes private draft metadata for the current viewer.
type DraftSource interface {
	Drafts(context.Context, int) ([]domain.PageDraft, error)
}

// PageListCapabilities exposes only the bounded page-list operations implemented by source.
func PageListCapabilities(source any) map[string]plugin.Capability {
	result := make(map[string]plugin.Capability)
	if source == nil {
		return result
	}

	decode := func(data json.RawMessage) (sdk.PageListQuery, error) {
		var request sdk.PageListQuery
		if err := json.Unmarshal(data, &request); err != nil || !validPageListQuery(request) {
			return sdk.PageListQuery{}, errors.New("invalid page list query")
		}
		return request, nil
	}

	if pages, ok := source.(RecentPagesSource); ok {
		result["pages.recent"] = func(ctx context.Context, data json.RawMessage) (any, error) {
			request, err := decode(data)
			if err != nil {
				return nil, err
			}
			return pageValues(pages.Recent(ctx, request.Limit))
		}
	}
	if pages, ok := source.(RecentViewedSource); ok {
		result["pages.recent-viewed"] = func(ctx context.Context, data json.RawMessage) (any, error) {
			request, err := decode(data)
			if err != nil {
				return nil, err
			}
			return pageValues(pages.RecentViewed(ctx, request.Limit))
		}
	}
	if pages, ok := source.(FavoritePagesSource); ok {
		result["pages.favorites"] = func(ctx context.Context, data json.RawMessage) (any, error) {
			request, err := decode(data)
			if err != nil {
				return nil, err
			}
			return pageValues(pages.Favorites(ctx, request.Limit))
		}
	}
	if pages, ok := source.(PopularPagesSource); ok {
		result["pages.popular"] = func(ctx context.Context, data json.RawMessage) (any, error) {
			request, err := decode(data)
			if err != nil {
				return nil, err
			}
			return pageValues(pages.Popular(ctx, request.Limit))
		}
	}
	if edits, ok := source.(RecentEditsSource); ok {
		result["pages.recent-edits"] = func(ctx context.Context, data json.RawMessage) (any, error) {
			request, err := decode(data)
			if err != nil {
				return nil, err
			}
			records, err := edits.RecentEdited(ctx, request.Limit)
			if err != nil {
				return nil, err
			}
			values := make([]sdk.RecentEdit, 0, len(records))
			for _, record := range records {
				values = append(values, sdk.RecentEdit{Page: PageValue(record.Page), RevisionMessage: record.RevisionMessage})
			}
			return values, nil
		}
	}

	return result
}

// DraftCapabilities exposes bounded private draft metadata without editor values.
func DraftCapabilities(source DraftSource) map[string]plugin.Capability {
	if source == nil {
		return nil
	}
	return map[string]plugin.Capability{
		"drafts.list": func(ctx context.Context, data json.RawMessage) (any, error) {
			var request sdk.PageListQuery
			if err := json.Unmarshal(data, &request); err != nil || !validPageListQuery(request) {
				return nil, errors.New("invalid page list query")
			}
			drafts, err := source.Drafts(ctx, request.Limit)
			if err != nil {
				return nil, err
			}
			values := make([]sdk.PageDraft, 0, len(drafts))
			for _, draft := range drafts {
				values = append(values, sdk.PageDraft{
					Key:       draft.Key,
					PageID:    draft.PageID,
					PageSlug:  draft.PageSlug,
					Title:     draft.Title,
					Stale:     draft.Stale,
					UpdatedAt: draft.UpdatedAt,
				})
			}
			return values, nil
		},
	}
}

// MergeCapabilities combines independent capability sets. Later sets replace duplicate keys.
func MergeCapabilities(sets ...map[string]plugin.Capability) map[string]plugin.Capability {
	result := make(map[string]plugin.Capability)
	for _, set := range sets {
		for name, capability := range set {
			result[name] = capability
		}
	}
	return result
}

// pageValues converts pages into plugin capability values.
func pageValues(pages []domain.Page, err error) ([]sdk.Page, error) {
	if err != nil {
		return nil, err
	}
	result := make([]sdk.Page, 0, len(pages))
	for _, page := range pages {
		result = append(result, PageValue(page))
	}
	return result, nil
}

// validPageListQuery reports whether a page-list capability query is supported.
func validPageListQuery(request sdk.PageListQuery) bool {
	return request.Limit >= 1 && request.Limit <= 100
}
