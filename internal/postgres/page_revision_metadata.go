package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/kumbuka-me/kumbuka/pkg/domain"
	"github.com/kumbuka-me/kumbuka/pkg/revision"
)

// PageSaveAuditTransactional declares that Store persists page-save audit events in the same transaction.
func (*Store) PageSaveAuditTransactional() {}

// snapshotPage converts the complete persisted page into canonical versioned metadata.
func snapshotPage(page domain.Page) revision.PageSnapshot {
	snapshot := revision.PageSnapshot{
		Slug: page.Slug, Title: page.Title, Icon: page.Icon, Language: page.Language,
		Tags: slices.Clone(page.Tags), Status: string(page.Status), OwnerGroupID: page.OwnerGroupID,
		ReviewIntervalDays: page.ReviewIntervalDays, DeprecatedTarget: page.DeprecatedTarget,
		Properties: make(map[string]string, len(page.Properties)), ReviewedAt: page.LastReviewedAt,
	}
	slices.Sort(snapshot.Tags)
	for _, group := range page.Groups {
		snapshot.GroupIDs = append(snapshot.GroupIDs, group.ID)
	}
	slices.Sort(snapshot.GroupIDs)
	for _, property := range page.Properties {
		snapshot.Properties[property.Key] = property.Value
	}
	return snapshot
}

// previousPageForSave locks the edited row so the comparison and revision are atomic.
func previousPageForSave(ctx context.Context, tx pgx.Tx, record pageSaveRecord) (*domain.Page, error) {
	slug := pageRecordLookupSlug(record)
	_, deleted, found, err := loadPageRecordForUpdate(ctx, tx, slug)
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, nil
	}
	if deleted {
		return nil, domain.ErrPageInBin
	}
	previous, err := getPage(ctx, tx, slug)
	if err != nil {
		return nil, err
	}
	return &previous, nil
}

// persistPageRevision compares the final committed state and appends a revision and audit event only for real changes.
// Returning false signals a save that only refreshed derived rendering artifacts.
func persistPageRevision(ctx context.Context, tx pgx.Tx, before *domain.Page, after domain.Page, message string, actorID int64) (bool, error) {
	snapshot := snapshotPage(after)
	var previousSnapshot *revision.PageSnapshot
	if before != nil {
		prior := snapshotPage(*before)
		previousSnapshot = &prior
	}
	changed := revision.ChangedFields(previousSnapshot, snapshot)
	contentChanged := before == nil || before.Markdown != after.Markdown
	if !contentChanged && len(changed) == 0 {
		return false, nil
	}

	action := "page.updated"
	if before == nil {
		action = "page.created"
	} else if before.Slug != after.Slug {
		action = "page.renamed"
	}
	if strings.HasPrefix(message, "Restore revision ") {
		action = "page.revision_restored"
	}

	summary := pageRevisionSummary(contentChanged, changed, action)
	if previousSnapshot != nil {
		summary = pageRevisionAuditSummary(contentChanged, *previousSnapshot, snapshot, action)
	}
	if strings.TrimSpace(message) == "" {
		message = summary
	}
	number, err := insertPageRevision(ctx, tx, after.ID, after.Markdown, message, actorID, snapshot, changed)
	if err != nil {
		return false, err
	}
	_, err = tx.Exec(ctx, `
INSERT INTO audit_events(user_id,action,object_type,object_key,detail,revision_number)
VALUES(NULLIF($1,0),$2,'page',$3,$4,$5)`, actorID, action, after.Slug, fmt.Sprintf("r%d · %s", number, summary), number)
	return true, err
}

// pageRevisionSummary describes changed field categories without leaking metadata or Markdown values.
func pageRevisionSummary(contentChanged bool, metadata []string, action string) string {
	if action == "page.created" {
		return "Page created"
	}
	parts := make([]string, 0, 2)
	if contentChanged {
		parts = append(parts, "Markdown changed")
	}
	if len(metadata) > 0 {
		parts = append(parts, "Changed: "+strings.Join(metadata, ", "))
	}
	if action == "page.revision_restored" {
		parts = append([]string{"Revision restored"}, parts...)
	}
	return strings.Join(parts, " · ")
}

// pageRevisionAuditSummary adds bounded before/after details for safe metadata fields.
func pageRevisionAuditSummary(contentChanged bool, before, after revision.PageSnapshot, action string) string {
	summary := pageRevisionSummary(contentChanged, revision.ChangedFields(&before, after), action)
	differences := revision.CompareSnapshots(before, after)
	if len(differences) == 0 {
		return summary
	}
	items := make([]string, 0, len(differences))
	for _, diff := range differences {
		if diff.Name == "properties" {
			continue
		}
		items = append(items, diff.Name+": "+diff.Before+" → "+diff.After)
	}
	if len(items) == 0 {
		return summary
	}
	return summary + " · " + strings.Join(items, "; ")
}

// insertPageRevision writes the immutable Markdown and metadata snapshot and returns its revision number.
func insertPageRevision(ctx context.Context, tx pgx.Tx, pageID int64, markdown, message string, actorID int64, snapshot revision.PageSnapshot, changed []string) (int, error) {
	var number int
	if err := tx.QueryRow(ctx, `SELECT coalesce(max(revision_number),0)+1 FROM page_revisions WHERE page_id=$1`, pageID).Scan(&number); err != nil {
		return 0, err
	}
	payload, err := json.Marshal(snapshot)
	if err != nil {
		return 0, fmt.Errorf("encode revision metadata: %w", err)
	}
	if changed == nil {
		changed = []string{}
	}
	_, err = tx.Exec(ctx, `
INSERT INTO page_revisions(page_id,revision_number,markdown_content,created_by,message,metadata_snapshot,metadata_changes)
VALUES($1,$2,$3,$4,$5,$6::jsonb,$7)`, pageID, number, markdown, actorID, message, payload, changed)
	return number, err
}

// decodeRevisionSnapshot restores optional snapshots without inventing history for older revisions.
func decodeRevisionSnapshot(record *revision.Revision, data []byte) error {
	if len(data) == 0 {
		return nil
	}
	var snapshot revision.PageSnapshot
	if err := json.Unmarshal(data, &snapshot); err != nil {
		return fmt.Errorf("decode revision metadata: %w", err)
	}
	record.Snapshot = &snapshot
	return nil
}
