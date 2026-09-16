package service

import (
	"context"
	"errors"
	"testing"

	"github.com/kumbuka-me/kumbuka/pkg/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type preferenceRepositoryStub struct {
	preferenceRepository
	saved domain.UserPreferences
}

func (s *preferenceRepositoryStub) SavePreferences(
	_ context.Context,
	_ int64,
	preferences domain.UserPreferences,
) error {
	s.saved = preferences
	return nil
}

func TestSavePreferencesValidatesTypographySize(t *testing.T) {
	t.Parallel()

	t.Run("allows application default", func(t *testing.T) {
		t.Parallel()

		repository := &preferenceRepositoryStub{}
		preferences := NewPreferences(repository)

		err := preferences.SavePreferences(context.Background(), 7, domain.UserPreferences{
			NavigationStyle:   domain.NavigationStyleSidebar,
			NavigationDensity: domain.NavigationDensityComfortable,
			SidebarWidth:      domain.DefaultSidebarWidth,
		})

		require.NoError(t, err)
		assert.Empty(t, repository.saved.TypographySize)
	})

	t.Run("allows personal preset", func(t *testing.T) {
		t.Parallel()

		repository := &preferenceRepositoryStub{}
		preferences := NewPreferences(repository)

		err := preferences.SavePreferences(context.Background(), 7, domain.UserPreferences{
			NavigationStyle:   domain.NavigationStyleSidebar,
			NavigationDensity: domain.NavigationDensityComfortable,
			TypographySize:    domain.TypographySizeLarge,
			SidebarWidth:      domain.DefaultSidebarWidth,
		})

		require.NoError(t, err)
		assert.Equal(t, domain.TypographySizeLarge, repository.saved.TypographySize)
	})

	t.Run("rejects unknown preset", func(t *testing.T) {
		t.Parallel()

		preferences := NewPreferences(&preferenceRepositoryStub{})
		err := preferences.SavePreferences(context.Background(), 7, domain.UserPreferences{
			NavigationStyle:   domain.NavigationStyleSidebar,
			NavigationDensity: domain.NavigationDensityComfortable,
			TypographySize:    "huge",
			SidebarWidth:      domain.DefaultSidebarWidth,
		})

		validation, ok := errors.AsType[*domain.ValidationError](err)
		require.True(t, ok)
		assert.Equal(t, "typography_size", validation.Fields[0].Field)
		assert.Equal(t, "Choose a valid typography size.", validation.UserMessage())
	})
}
