package pages

import (
	"bytes"
	"context"
	"errors"
	"github.com/kumbuka-me/kumbuka/pkg/domain"
	"github.com/stretchr/testify/require"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"
)

type failingPageViewRecorder struct{}

func (failingPageViewRecorder) RecordView(context.Context, string, int64) error {
	return errors.New("view history unavailable")
}

func TestRecordPageViewReportsPersistenceFailure(t *testing.T) {
	t.Parallel()

	var output bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&output, nil))

	RecordView(context.Background(), logger, failingPageViewRecorder{}, "guide", 42)

	logs := output.String()
	assert.Contains(t, logs, `"event":"page_view_record_failed"`)
	assert.Contains(t, logs, `"slug":"guide"`)
	assert.Contains(t, logs, `"user_id":42`)
	assert.Contains(t, logs, "view history unavailable")
}

// viewAccessFake counts resource checks without HTTP or persistence.
type viewAccessFake struct {
	allowed   bool
	calls     int
	bulkCalls int
	err       error
}

func (f *viewAccessFake) CanView(context.Context, domain.User, string) (bool, error) {
	f.calls++
	return f.allowed, f.err
}
func (f *viewAccessFake) CanEdit(context.Context, domain.User, string) (bool, error) {
	return false, nil
}
func (f *viewAccessFake) FilterPages(_ context.Context, _ domain.User, pages []domain.Page) ([]domain.Page, error) {
	f.bulkCalls++
	if f.err != nil {
		return nil, f.err
	}
	result := make([]domain.Page, 0, len(pages))
	for _, page := range pages {
		if page.Slug != "private" {
			result = append(result, page)
		}
	}
	return result, nil
}

// aliasReadFake supplies only the two reads an alias resolution needs.
type aliasReadFake struct {
	viewRepository
	alias string
	err   error
}

func (f aliasReadFake) GetPage(context.Context, string) (domain.Page, error) {
	return domain.Page{}, domain.ErrNotFound
}
func (f aliasReadFake) ResolvePageAlias(context.Context, string) (string, error) {
	return f.alias, f.err
}

func TestViewDenialPrecedesPersistence(t *testing.T) {
	t.Parallel()
	policy := &viewAccessFake{}
	_, err := NewView(nil, policy, nil, nil).Execute(context.Background(), domain.User{ID: 5}, "private")
	require.ErrorIs(t, err, domain.ErrNotFound)
	assert.Equal(t, 1, policy.calls)
}

func TestViewAliasAndFailure(t *testing.T) {
	t.Parallel()
	policy := &viewAccessFake{allowed: true}
	result, err := NewView(aliasReadFake{alias: "current"}, policy, nil, nil).Execute(context.Background(), domain.User{ID: 5}, "old")
	require.NoError(t, err)
	assert.Equal(t, "current", result.Alias)
	assert.Equal(t, 1, policy.calls)
	failure := errors.New("lookup failed")
	_, err = NewView(aliasReadFake{err: failure}, policy, nil, nil).Execute(context.Background(), domain.User{}, "old")
	require.ErrorIs(t, err, failure)
}

func TestCollectionAccessIsBulk(t *testing.T) {
	t.Parallel()
	policy := &viewAccessFake{}
	graph := domain.KnowledgeGraph{Nodes: []domain.GraphNode{{Slug: "open"}, {Slug: "private"}}, Edges: []domain.GraphEdge{{Source: "open", Target: "private"}}}
	filtered, err := VisibleKnowledgeGraph(context.Background(), policy, domain.User{}, graph)
	require.NoError(t, err)
	assert.Len(t, filtered.Nodes, 1)
	assert.Empty(t, filtered.Edges)
	assert.Len(t, graph.Nodes, 2)
	edits, err := visibleRecentEdits(context.Background(), policy, domain.User{}, []domain.RecentEdit{{Slug: "open"}, {Slug: "private"}})
	require.NoError(t, err)
	assert.Len(t, edits, 1)
	assert.Equal(t, 2, policy.bulkCalls)
	assert.Zero(t, policy.calls)
}
