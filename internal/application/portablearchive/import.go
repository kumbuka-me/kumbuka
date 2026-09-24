// Package portablearchive coordinates portable archive restoration across application capabilities.
package portablearchive

import (
	"context"
	"fmt"
	"net/url"
	"path"
	"path/filepath"
	"slices"
	"sort"
	"strconv"
	"strings"

	"github.com/kumbuka-me/kumbuka/internal/application/pages"
	"github.com/kumbuka-me/kumbuka/internal/markdownurl"
	"github.com/kumbuka-me/kumbuka/internal/portable"
	"github.com/kumbuka-me/kumbuka/pkg/domain"
)

// Pages runs a portable import transaction and persists pages inside it.
type Pages interface {
	RunPortableImport(context.Context, domain.User, func(context.Context) (int, error)) (int, error)
	ImportPortablePages(context.Context, []pages.PortableImportedPage, domain.User) (int, error)
}

// Media recreates archived resources using the standard upload validation rules.
type Media interface {
	UploadImage(context.Context, string, []byte, domain.User) (domain.Image, error)
	UploadAttachment(context.Context, string, []byte, domain.User) (domain.Attachment, error)
}

// Groups resolves and creates collaboration groups inside the import transaction.
type Groups interface {
	Groups(context.Context) ([]domain.Group, error)
	CreateGroup(context.Context, string) (domain.Group, error)
}

// Restore commits all archived resources, groups, and pages together.
func Restore(ctx context.Context, archive portable.Archive, pageService Pages, mediaService Media, groupService Groups, actor domain.User) (int, error) {
	return pageService.RunPortableImport(ctx, actor, func(transactionContext context.Context) (int, error) {
		return restore(transactionContext, archive, pageService, mediaService, groupService, actor)
	})
}

// restore recreates resources and groups, then persists pages on the same transaction context.
func restore(ctx context.Context, archive portable.Archive, pageService Pages, mediaService Media, groupService Groups, actor domain.User) (int, error) {
	replacements := make(map[string]string, len(archive.Manifest.Media)+len(archive.Manifest.Attachments))
	for _, resource := range archive.Manifest.Media {
		image, err := mediaService.UploadImage(ctx, resource.Filename, archive.Resources[resource.Path], actor)
		if err != nil {
			return 0, fmt.Errorf("restore image %q: %w", resource.Path, err)
		}
		replacements[resource.Path] = resourceURL("media", image.ID, image.Filename)
	}
	for _, resource := range archive.Manifest.Attachments {
		attachment, err := mediaService.UploadAttachment(ctx, resource.Filename, archive.Resources[resource.Path], actor)
		if err != nil {
			return 0, fmt.Errorf("restore attachment %q: %w", resource.Path, err)
		}
		replacements[resource.Path] = resourceURL("attachments", attachment.ID, attachment.Filename)
	}

	groupIDs, err := ensureGroups(ctx, groupService, archive.Pages)
	if err != nil {
		return 0, err
	}
	imported := make([]pages.PortableImportedPage, 0, len(archive.Pages))
	for _, pageData := range archive.Pages {
		markdown, err := RestoreResourceReferences(pageData.Entry.Markdown, pageData.Markdown, replacements)
		if err != nil {
			return 0, err
		}
		groups := make([]int64, 0, len(pageData.Metadata.Groups))
		seen := map[int64]bool{}
		for _, name := range pageData.Metadata.Groups {
			id := groupIDs[groupKey(name)]
			if id > 0 && !seen[id] {
				groups = append(groups, id)
				seen[id] = true
			}
		}
		imported = append(imported, pages.PortableImportedPage{
			Slug: pageData.Metadata.Slug, Title: pageData.Metadata.Title,
			Icon: pageData.Metadata.Icon, Language: pageData.Metadata.Language,
			Markdown: markdown, Tags: slices.Clone(pageData.Metadata.Tags), GroupIDs: groups,
			Status: pageData.Metadata.Status, OwnerGroupID: groupIDs[groupKey(pageData.Metadata.OwnerGroup)],
			ReviewIntervalDays: pageData.Metadata.ReviewIntervalDays,
			DeprecatedTarget: pageData.Metadata.DeprecatedTarget,
			Properties: cloneProperties(pageData.Metadata.Properties),
		})
	}
	return pageService.ImportPortablePages(ctx, imported, actor)
}

// resourceURL constructs a stable authenticated URL for an imported resource.
func resourceURL(kind string, id int64, filename string) string {
	return "/" + kind + "/" + strconv.FormatInt(id, 10) + "/" + url.PathEscape(filename)
}

// ensureGroups resolves existing names and creates missing groups in deterministic order.
func ensureGroups(ctx context.Context, service Groups, archived []portable.Page) (map[string]int64, error) {
	groups, err := service.Groups(ctx)
	if err != nil {
		return nil, err
	}
	ids := make(map[string]int64, len(groups))
	for _, group := range groups {
		ids[groupKey(group.Name)] = group.ID
	}
	missing := map[string]string{}
	for _, pageData := range archived {
		names := append(slices.Clone(pageData.Metadata.Groups), pageData.Metadata.OwnerGroup)
		for _, name := range names {
			name = strings.TrimSpace(name)
			if name != "" && ids[groupKey(name)] == 0 {
				missing[groupKey(name)] = name
			}
		}
	}
	keys := make([]string, 0, len(missing))
	for key := range missing {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		group, err := service.CreateGroup(ctx, missing[key])
		if err != nil {
			return nil, fmt.Errorf("create imported group %q: %w", missing[key], err)
		}
		ids[key] = group.ID
	}
	return ids, nil
}

// groupKey identifies the case-insensitive group name used for archive mapping.
func groupKey(name string) string { return strings.ToLower(strings.TrimSpace(name)) }

// RestoreResourceReferences changes only archive-relative Markdown URL destinations.
func RestoreResourceReferences(markdownPath, markdown string, replacements map[string]string) (string, error) {
	urls := make(map[string]string, len(replacements))
	from := filepath.FromSlash(path.Dir(markdownPath))
	for resourcePath, replacement := range replacements {
		relative, err := filepath.Rel(from, filepath.FromSlash(resourcePath))
		if err != nil {
			return "", err
		}
		urls[filepath.ToSlash(relative)] = replacement
	}
	return markdownurl.Rewrite(markdown, func(destination string) (string, bool, error) {
		replacement, ok := urls[destination]
		return replacement, ok, nil
	})
}

// cloneProperties prevents archive metadata from aliasing a persisted page input.
func cloneProperties(properties map[string]string) map[string]string {
	if len(properties) == 0 {
		return map[string]string{}
	}
	clone := make(map[string]string, len(properties))
	for key, value := range properties {
		clone[key] = value
	}
	return clone
}
