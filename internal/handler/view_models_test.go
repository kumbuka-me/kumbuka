package handler

import (
	"testing"

	"github.com/kumbuka-me/kumbuka/pkg/navigation"
	"github.com/stretchr/testify/assert"
)

func TestPagePathOptions(t *testing.T) {
	t.Parallel()

	t.Run("flattens navigation with breadcrumb labels", func(t *testing.T) {
		t.Parallel()

		tree := []navigation.Node{
			{
				Slug: "platforms", Title: "Platforms", Children: []navigation.Node{
					{Slug: "platforms/containers", Title: "Containers"},
					{Slug: "platforms/kubernetes", Title: "Kubernetes"},
				},
			},
		}

		assert.Equal(t, []pagePathOption{
			{Slug: "platforms", Label: "Platforms"},
			{Slug: "platforms/containers", Label: "Platforms / Containers"},
			{Slug: "platforms/kubernetes", Label: "Platforms / Kubernetes"},
		}, pagePathOptions(tree, ""))
	})

	t.Run("excludes selected page and its subtree", func(t *testing.T) {
		t.Parallel()

		tree := []navigation.Node{
			{
				Slug: "platforms", Title: "Platforms", Children: []navigation.Node{
					{
						Slug: "platforms/containers", Title: "Containers", Children: []navigation.Node{
							{Slug: "platforms/containers/docker", Title: "Docker"},
						},
					},
					{Slug: "platforms/kubernetes", Title: "Kubernetes"},
				},
			},
		}

		assert.Equal(t, []pagePathOption{
			{Slug: "platforms", Label: "Platforms"},
			{Slug: "platforms/kubernetes", Label: "Platforms / Kubernetes"},
		}, pagePathOptions(tree, "platforms/containers"))
	})

	t.Run("returns no options for empty navigation", func(t *testing.T) {
		t.Parallel()

		assert.Empty(t, pagePathOptions(nil, ""))
	})
}

func TestHasPagePathOption(t *testing.T) {
	t.Parallel()

	options := []pagePathOption{
		{Slug: "platforms", Label: "Platforms"},
		{Slug: "platforms/kubernetes", Label: "Platforms / Kubernetes"},
	}

	t.Run("allows root path", func(t *testing.T) {
		t.Parallel()

		assert.True(t, hasPagePathOption(options, ""))
	})

	t.Run("finds existing path", func(t *testing.T) {
		t.Parallel()

		assert.True(t, hasPagePathOption(options, "platforms/kubernetes"))
	})

	t.Run("rejects missing path", func(t *testing.T) {
		t.Parallel()

		assert.False(t, hasPagePathOption(options, "platforms/nomad"))
	})
}
