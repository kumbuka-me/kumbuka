package pages

import (
	"context"
	"testing"

	"github.com/kumbuka-me/kumbuka/pkg/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// directoryRepositoryStub provides controllable directory repository behavior for tests.
type directoryRepositoryStub struct {
	// aliases configures or records the aliases value used by the fixture.
	aliases map[string]string
}

func (s directoryRepositoryStub) PageAliases(context.Context) (map[string]string, error) {
	return s.aliases, nil
}

func (directoryRepositoryStub) PageInventory(context.Context) ([]domain.Page, error) {
	return nil, nil
}

// directoryAccessStub provides controllable directory access behavior for tests.
type directoryAccessStub struct {
	// visible configures or records the visible value used by the fixture.
	visible []domain.Page
	// pages records the pages observed by the test double.
	pages []domain.Page
	// actor records the actor observed by the test double.
	actor domain.User
}

func (*directoryAccessStub) CanView(context.Context, domain.User, string) (bool, error) {
	return false, nil
}

func (*directoryAccessStub) CanEdit(context.Context, domain.User, string) (bool, error) {
	return false, nil
}

func (s *directoryAccessStub) FilterPages(_ context.Context, actor domain.User, pages []domain.Page) ([]domain.Page, error) {
	s.actor = actor
	s.pages = append([]domain.Page(nil), pages...)
	return append([]domain.Page(nil), s.visible...), nil
}

func TestPageAliasesForFiltersRestrictedTargets(t *testing.T) {
	t.Parallel()

	actor := domain.User{ID: 42, Role: domain.UserRoleEditor}
	access := &directoryAccessStub{visible: []domain.Page{{Slug: "public"}}}
	directory := NewDirectory(directoryRepositoryStub{aliases: map[string]string{
		"old-public":   "public",
		"older-public": "public",
		"old-secret":   "secret",
	}}, access)

	aliases, err := directory.PageAliasesFor(context.Background(), actor)

	require.NoError(t, err)
	assert.Equal(t, map[string]string{
		"old-public":   "public",
		"older-public": "public",
	}, aliases)
	assert.Equal(t, actor, access.actor)
	assert.ElementsMatch(t, []domain.Page{{Slug: "public"}, {Slug: "secret"}}, access.pages)
}
