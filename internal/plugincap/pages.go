// Package plugincap adapts already-authorized application data to public wire
// values. It does not fetch an unrestricted store or expose domain objects.
package plugincap

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/kumbuka-me/kumbuka/internal/domain"
	"github.com/kumbuka-me/kumbuka/internal/icons"
	"github.com/kumbuka-me/kumbuka/internal/navigation"
	"github.com/kumbuka-me/kumbuka/internal/plugin"
	"github.com/kumbuka-me/kumbuka/internal/revision"
	"github.com/kumbuka-me/sdk"
)

// Source is the minimal authorized page catalog required by plugin page capabilities.
type Source interface {
	// Search queries the authorized source and converts results to public plugin values.
	Search(context.Context, string, int) ([]domain.Page, error)
	// GetPage returns one authorized page as a public plugin value.
	GetPage(context.Context, string) (domain.Page, error)
}

// LinkSource exposes authorized wiki-link relationships when available.
type LinkSource interface {
	Backlinks(context.Context, string) ([]domain.Page, error)
	PageLinks(context.Context, string) ([]domain.PageLink, error)
}

// RevisionSource exposes authorized page revisions when available.
type RevisionSource interface {
	Revisions(context.Context, string) ([]revision.Revision, error)
}

// LatestRevisionSource exposes an efficient newest-revision lookup.
type LatestRevisionSource interface {
	LatestRevision(context.Context, string) (revision.Revision, int, error)
}

// Pages adapts an already-authorized page catalog to the public plugin capability API.
type Pages struct {
	// Source is the authorized page catalog used for plugin lookups.
	Source Source
}

// Search queries the authorized source and converts results to public plugin values.
func (p Pages) Search(ctx context.Context, query string, limit int) ([]sdk.Page, error) {
	pages, err := p.Source.Search(ctx, query, limit)
	if err != nil {
		return nil, err
	}

	result := make([]sdk.Page, 0, len(pages))
	for _, page := range pages {
		result = append(result, PageValue(page))
	}

	return result, nil
}

// GetPage returns one authorized page as a public plugin value.
func (p Pages) GetPage(ctx context.Context, slug string) (sdk.Page, error) {
	page, err := p.Source.GetPage(ctx, slug)
	return PageValue(page), err
}

// Content returns authorized stored Markdown without exposing internal domain values.
func (p Pages) Content(ctx context.Context, slug string) (sdk.PageContent, error) {
	page, err := p.Source.GetPage(ctx, slug)
	if err != nil {
		return sdk.PageContent{}, err
	}
	return sdk.PageContent{Slug: page.Slug, Markdown: page.Markdown}, nil
}

// Links returns authorized incoming and outgoing wiki-link relationships.
func (p Pages) Links(ctx context.Context, source LinkSource, slug string) (sdk.PageLinks, error) {
	backlinks, err := source.Backlinks(ctx, slug)
	if err != nil {
		return sdk.PageLinks{}, err
	}
	outgoing, err := source.PageLinks(ctx, slug)
	if err != nil {
		return sdk.PageLinks{}, err
	}
	result := sdk.PageLinks{Outgoing: make([]sdk.PageLink, 0, len(outgoing))}
	for _, page := range backlinks {
		result.Backlinks = append(result.Backlinks, PageValue(page))
	}
	for _, link := range outgoing {
		result.Outgoing = append(result.Outgoing, sdk.PageLink{TargetSlug: link.TargetSlug, TargetTitle: link.TargetTitle, Exists: link.Exists})
	}
	return result, nil
}

// Revisions returns bounded public revision metadata without stored page bodies.
func (p Pages) Revisions(ctx context.Context, query sdk.RevisionQuery) (sdk.RevisionHistory, error) {
	if latest, ok := p.Source.(LatestRevisionSource); ok && query.Limit == 1 {
		record, count, err := latest.LatestRevision(ctx, query.Slug)
		if err != nil {
			return sdk.RevisionHistory{}, err
		}
		if count == 0 {
			return sdk.RevisionHistory{}, nil
		}
		record = revision.Analyze(record)
		return sdk.RevisionHistory{Count: count, Revisions: []sdk.Revision{revisionValue(record)}}, nil
	}
	source, ok := p.Source.(RevisionSource)
	if !ok {
		return sdk.RevisionHistory{}, errors.New("page revisions are unavailable")
	}
	records, err := source.Revisions(ctx, query.Slug)
	if err != nil {
		return sdk.RevisionHistory{}, err
	}
	count := len(records)
	if len(records) > query.Limit {
		records = records[:query.Limit]
	}
	result := sdk.RevisionHistory{Count: count, Revisions: make([]sdk.Revision, 0, len(records))}
	for _, record := range records {
		result.Revisions = append(result.Revisions, revisionValue(revision.Analyze(record)))
	}
	return result, nil
}

func revisionValue(record revision.Revision) sdk.Revision {
	return sdk.Revision{Number: record.Number, Author: record.Author, CreatedAt: record.CreatedAt, Message: record.Message, AddedLines: record.AddedLines, RemovedLines: record.RemovedLines}
}

// PageValue converts an internal page record into its public plugin representation.
func PageValue(page domain.Page) sdk.Page {
	result := sdk.Page{
		Slug:       page.Slug,
		Title:      page.Title,
		Status:     page.Status,
		OwnerGroup: page.OwnerGroup,
		UpdatedAt:  page.UpdatedAt,
		Author:     page.Author,
		Tags:       page.Tags,
		ViewCount:  page.ViewCount,
	}

	for _, property := range page.Properties {
		result.Properties = append(result.Properties, sdk.Property{Key: property.Key, Value: property.Value})
	}

	return result
}

// Navigation exposes prepared navigation nodes through the public plugin capability API.
func Navigation(nodes []navigation.Node, pageURL func(string) string) []sdk.NavigationNode {
	result := make([]sdk.NavigationNode, 0, len(nodes))

	for _, node := range nodes {
		item := sdk.NavigationNode{Title: node.Title, Icon: node.Icon, Page: node.Page, Children: Navigation(node.Children, pageURL)}
		if node.Page {
			item.URL = pageURL(node.Slug)
		}
		result = append(result, item)
	}

	return result
}

// Capabilities builds the render-scoped capability map supplied to plugins.
func Capabilities(source Source, nodes []sdk.NavigationNode, catalogs ...*icons.Catalog) map[string]plugin.Capability {
	catalog := icons.Builtin()
	if len(catalogs) > 0 && catalogs[0] != nil {
		catalog = catalogs[0]
	}
	result := map[string]plugin.Capability{
		"pages.navigation": func(context.Context, json.RawMessage) (any, error) { return nodes, nil },
		"icons.render": func(_ context.Context, data json.RawMessage) (any, error) {
			var request sdk.IconRequest
			if err := json.Unmarshal(data, &request); err != nil || !validIconRequest(request) {
				return nil, errors.New("invalid icon request")
			}
			return string(catalog.SVG(request.Name, request.Size)), nil
		},
	}

	if source != nil {
		pages := Pages{source}
		result["pages.get"] = func(ctx context.Context, data json.RawMessage) (any, error) {
			var request sdk.PageRef
			if err := json.Unmarshal(data, &request); err != nil || !validPageRef(request) {
				return nil, errors.New("invalid page reference")
			}
			return pages.GetPage(ctx, request.Slug)
		}
		result["pages.content"] = func(ctx context.Context, data json.RawMessage) (any, error) {
			var request sdk.PageRef
			if err := json.Unmarshal(data, &request); err != nil || !validPageRef(request) {
				return nil, errors.New("invalid page reference")
			}
			return pages.Content(ctx, request.Slug)
		}
		result["pages.search"] = func(ctx context.Context, data json.RawMessage) (any, error) {
			var request sdk.PageQuery
			if err := json.Unmarshal(data, &request); err != nil || !validPageQuery(request) {
				return nil, errors.New("invalid page query")
			}
			return pages.Search(ctx, request.Query, request.Limit)
		}
		if links, ok := source.(LinkSource); ok {
			result["pages.links"] = func(ctx context.Context, data json.RawMessage) (any, error) {
				var request sdk.PageRef
				if err := json.Unmarshal(data, &request); err != nil || !validPageRef(request) {
					return nil, errors.New("invalid page reference")
				}
				return pages.Links(ctx, links, request.Slug)
			}
		}
		if _, ok := source.(RevisionSource); ok {
			result["pages.revisions"] = func(ctx context.Context, data json.RawMessage) (any, error) {
				var request sdk.RevisionQuery
				if err := json.Unmarshal(data, &request); err != nil || !validRevisionQuery(request) {
					return nil, errors.New("invalid revision query")
				}
				return pages.Revisions(ctx, request)
			}
		} else if _, ok := source.(LatestRevisionSource); ok {
			result["pages.revisions"] = func(ctx context.Context, data json.RawMessage) (any, error) {
				var request sdk.RevisionQuery
				if err := json.Unmarshal(data, &request); err != nil || !validRevisionQuery(request) || request.Limit != 1 {
					return nil, errors.New("invalid revision query")
				}
				return pages.Revisions(ctx, request)
			}
		}
	}

	return result
}

// validIconRequest reports whether an icon capability request stays within supported bounds.
func validIconRequest(request sdk.IconRequest) bool {
	return len(request.Name) <= 128 && request.Size >= 1 && request.Size <= 256
}

// validPageRef reports whether a page reference contains a bounded non-empty slug.
func validPageRef(request sdk.PageRef) bool {
	return len(request.Slug) > 0 && len(request.Slug) <= 4096
}

// validPageQuery reports whether a page search request stays within supported bounds.
func validPageQuery(request sdk.PageQuery) bool {
	return len(request.Query) <= 4096 && request.Limit >= 1 && request.Limit <= 100
}

func validRevisionQuery(request sdk.RevisionQuery) bool {
	return len(request.Slug) > 0 && len(request.Slug) <= 4096 && request.Limit >= 1 && request.Limit <= 100
}

// SharedPages constrains anonymous capability calls to the explicitly shared
// page. Knowing another slug or matching it in search never grants access.
type SharedPages struct {
	// Source is the authorized page catalog used for plugin lookups.
	Source Source
	// Slug is the only page path this constrained source may expose.
	Slug string
}

// GetPage returns one authorized page as a public plugin value.
func (s SharedPages) GetPage(ctx context.Context, slug string) (domain.Page, error) {
	if slug != s.Slug {
		return domain.Page{}, errors.New("page unavailable")
	}

	return s.Source.GetPage(ctx, slug)
}

// Search queries the authorized source and converts results to public plugin values.
func (s SharedPages) Search(ctx context.Context, query string, limit int) ([]domain.Page, error) {
	pages, err := s.Source.Search(ctx, query, limit)
	if err != nil {
		return nil, err
	}

	var result []domain.Page
	for _, page := range pages {
		if page.Slug == s.Slug {
			result = append(result, page)
		}
	}

	return result, nil
}
