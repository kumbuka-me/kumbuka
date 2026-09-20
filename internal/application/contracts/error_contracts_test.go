package contracts

import (
	"context"
	"errors"
	"log/slog"
	"testing"

	"github.com/kumbuka-me/kumbuka/internal/application/groups"
	"github.com/kumbuka-me/kumbuka/internal/application/navigation"
	"github.com/kumbuka-me/kumbuka/internal/application/notifications"
	"github.com/kumbuka-me/kumbuka/internal/application/pages"
	"github.com/kumbuka-me/kumbuka/internal/application/preferences"
	"github.com/kumbuka-me/kumbuka/internal/application/search"
	"github.com/kumbuka-me/kumbuka/internal/application/templates"
	"github.com/kumbuka-me/kumbuka/internal/application/tokens"
	"github.com/kumbuka-me/kumbuka/internal/application/users"
	"github.com/kumbuka-me/kumbuka/pkg/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestKnownInputFailuresAreValidationErrors(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	// Nil repositories ensure invalid input is rejected before persistence.
	t.Run("notification id for mark read", func(t *testing.T) {
		t.Parallel()

		err := notifications.NewNotifications(nil).MarkNotificationRead(ctx, 1, 0)

		validation, ok := errors.AsType[*domain.ValidationError](err)
		require.True(t, ok)
		require.Len(t, validation.Fields, 1)
		assert.Equal(t, "notification", validation.Fields[0].Field)
	})

	t.Run("notification id for open", func(t *testing.T) {
		t.Parallel()

		_, err := notifications.NewNotifications(nil).OpenNotification(ctx, 1, 0)

		validation, ok := errors.AsType[*domain.ValidationError](err)
		require.True(t, ok)
		require.Len(t, validation.Fields, 1)
		assert.Equal(t, "notification", validation.Fields[0].Field)
	})

	t.Run("saved search name", func(t *testing.T) {
		t.Parallel()

		err := search.NewKnowledge(nil, nil).SaveSavedSearch(ctx, 1, 0, " ", "query", false)

		validation, ok := errors.AsType[*domain.ValidationError](err)
		require.True(t, ok)
		require.Len(t, validation.Fields, 1)
		assert.Equal(t, "name", validation.Fields[0].Field)
	})

	t.Run("saved search query", func(t *testing.T) {
		t.Parallel()

		err := search.NewKnowledge(nil, nil).SaveSavedSearch(ctx, 1, 0, "name", " ", false)

		validation, ok := errors.AsType[*domain.ValidationError](err)
		require.True(t, ok)
		require.Len(t, validation.Fields, 1)
		assert.Equal(t, "query", validation.Fields[0].Field)
	})

	t.Run("group name", func(t *testing.T) {
		t.Parallel()

		_, err := groups.NewGroups(nil).CreateGroup(ctx, " ")

		validation, ok := errors.AsType[*domain.ValidationError](err)
		require.True(t, ok)
		require.Len(t, validation.Fields, 1)
		assert.Equal(t, "name", validation.Fields[0].Field)
	})

	t.Run("create template name", func(t *testing.T) {
		t.Parallel()

		_, err := templates.NewTemplates(nil).CreatePageTemplate(ctx, templates.PageTemplateInput{Name: " "})

		validation, ok := errors.AsType[*domain.ValidationError](err)
		require.True(t, ok)
		require.Len(t, validation.Fields, 1)
		assert.Equal(t, "name", validation.Fields[0].Field)
	})

	t.Run("update template name", func(t *testing.T) {
		t.Parallel()

		err := templates.NewTemplates(nil).UpdatePageTemplate(ctx, 1, templates.PageTemplateInput{Name: " "})

		validation, ok := errors.AsType[*domain.ValidationError](err)
		require.True(t, ok)
		require.Len(t, validation.Fields, 1)
		assert.Equal(t, "name", validation.Fields[0].Field)
	})

	t.Run("same move path", func(t *testing.T) {
		t.Parallel()

		err := pages.NewMutations(nil, nil, nil, slog.Default()).Move(ctx, "/guide/", "guide", domain.MovePageOptions{}, domain.User{})

		validation, ok := errors.AsType[*domain.ValidationError](err)
		require.True(t, ok)
		require.Len(t, validation.Fields, 1)
		assert.Equal(t, "slug", validation.Fields[0].Field)
	})

	t.Run("move tree into itself", func(t *testing.T) {
		t.Parallel()

		err := pages.NewMutations(nil, nil, nil, slog.Default()).Move(ctx, "guide", "guide/child", domain.MovePageOptions{MoveChildren: true}, domain.User{})

		validation, ok := errors.AsType[*domain.ValidationError](err)
		require.True(t, ok)
		require.Len(t, validation.Fields, 1)
		assert.Equal(t, "slug", validation.Fields[0].Field)
	})

	t.Run("bulk move same path", func(t *testing.T) {
		t.Parallel()

		err := pages.NewBulk(nil, nil, nil, slog.Default()).Bulk(ctx, pages.BulkPageInput{Action: "move", Slugs: []string{"guide/child"}, Target: "guide"})

		validation, ok := errors.AsType[*domain.ValidationError](err)
		require.True(t, ok)
		require.Len(t, validation.Fields, 1)
		assert.Equal(t, "target", validation.Fields[0].Field)
	})
}

func TestAdditionalServiceValidationBeforePersistence(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	// Nil repositories ensure invalid input is rejected before persistence.
	t.Run("comment body", func(t *testing.T) {
		t.Parallel()

		_, err := pages.NewDiscussions(nil, nil, nil, nil, slog.Default()).AddComment(ctx, "page", 0, "", "", " ", domain.User{})

		validation, ok := errors.AsType[*domain.ValidationError](err)
		require.True(t, ok)
		require.Len(t, validation.Fields, 1)
		assert.Equal(t, "body", validation.Fields[0].Field)
	})

	t.Run("user role", func(t *testing.T) {
		t.Parallel()

		err := users.NewUsers(nil).UpdateUser(ctx, 1, "invalid", true, nil, nil)

		validation, ok := errors.AsType[*domain.ValidationError](err)
		require.True(t, ok)
		require.Len(t, validation.Fields, 1)
		assert.Equal(t, "role", validation.Fields[0].Field)
	})

	t.Run("token name", func(t *testing.T) {
		t.Parallel()

		_, err := tokens.NewTokens(nil).CreateToken(ctx, " ", 1, 1, nil)

		validation, ok := errors.AsType[*domain.ValidationError](err)
		require.True(t, ok)
		require.Len(t, validation.Fields, 1)
		assert.Equal(t, "name", validation.Fields[0].Field)
	})

	t.Run("sidebar width", func(t *testing.T) {
		t.Parallel()

		err := preferences.NewPreferences(nil).SetSidebarWidth(ctx, 1, -1)

		validation, ok := errors.AsType[*domain.ValidationError](err)
		require.True(t, ok)
		require.Len(t, validation.Fields, 1)
		assert.Equal(t, "sidebar_width", validation.Fields[0].Field)
	})

	t.Run("preference navigation style", func(t *testing.T) {
		t.Parallel()

		err := preferences.NewPreferences(nil).SavePreferences(ctx, 1, domain.UserPreferences{NavigationStyle: "invalid"})

		validation, ok := errors.AsType[*domain.ValidationError](err)
		require.True(t, ok)
		require.Len(t, validation.Fields, 1)
		assert.Equal(t, "navigation_style", validation.Fields[0].Field)
	})

	t.Run("preference density", func(t *testing.T) {
		t.Parallel()

		err := preferences.NewPreferences(nil).SavePreferences(ctx, 1, domain.UserPreferences{NavigationStyle: domain.NavigationStyleSidebar, NavigationDensity: "invalid"})

		validation, ok := errors.AsType[*domain.ValidationError](err)
		require.True(t, ok)
		require.Len(t, validation.Fields, 1)
		assert.Equal(t, "navigation_density", validation.Fields[0].Field)
	})

	t.Run("navigation icon", func(t *testing.T) {
		t.Parallel()

		err := navigation.NewNavigation(nil, nil).SetNavigationIcon(ctx, "page", "not-an-icon")

		validation, ok := errors.AsType[*domain.ValidationError](err)
		require.True(t, ok)
		require.Len(t, validation.Fields, 1)
		assert.Equal(t, "icon", validation.Fields[0].Field)
	})
}
