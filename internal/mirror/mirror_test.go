package mirror

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/kumbuka-me/kumbuka/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type repositoryStub struct {
	pages       []domain.Page
	images      []domain.Image
	imageData   map[int64]domain.ImageData
	attachments []domain.Attachment
	attachData  map[int64]domain.AttachmentData
}

func (s repositoryStub) PageInventory(context.Context) ([]domain.Page, error) { return s.pages, nil }

func (s repositoryStub) GetPage(_ context.Context, slug string) (domain.Page, error) {
	for _, page := range s.pages {
		if page.Slug == slug {
			return page, nil
		}
	}
	return domain.Page{}, domain.ErrNotFound
}
func (s repositoryStub) Images(context.Context) ([]domain.Image, error) { return s.images, nil }
func (s repositoryStub) ImageContent(_ context.Context, id int64) (domain.ImageData, error) {
	return s.imageData[id], nil
}

func (s repositoryStub) Attachments(context.Context) ([]domain.Attachment, error) {
	return s.attachments, nil
}

func (s repositoryStub) AttachmentContent(_ context.Context, id int64) (domain.AttachmentData, error) {
	return s.attachData[id], nil
}

func TestExport(t *testing.T) {
	t.Parallel()

	t.Run("writes deterministic page metadata and binary content", func(t *testing.T) {
		t.Parallel()

		stamp := time.Date(2026, time.September, 10, 12, 0, 0, 0, time.UTC)
		repository := repositoryStub{
			pages: []domain.Page{{
				ID: 2, Slug: "platform/runbook", Title: "Runbook", Markdown: "# Runbook\n", Status: "verified",
				CreatedAt: stamp, UpdatedAt: stamp, Tags: []string{"ops"},
			}},
			images:      []domain.Image{{ID: 7, Filename: "diagram.png"}},
			imageData:   map[int64]domain.ImageData{7: {Filename: "diagram.png", Data: []byte("png")}},
			attachments: []domain.Attachment{{ID: 9, Filename: "notes.txt"}},
			attachData:  map[int64]domain.AttachmentData{9: {Attachment: domain.Attachment{ID: 9, Filename: "notes.txt"}, Data: []byte("notes")}},
		}
		output := filepath.Join(t.TempDir(), "mirror")

		require.NoError(t, Export(context.Background(), repository, output))

		markdown, err := os.ReadFile(filepath.Join(output, "pages", "platform", "runbook.md"))
		require.NoError(t, err)
		assert.Equal(t, "# Runbook\n", string(markdown))

		metadata, err := os.ReadFile(filepath.Join(output, "metadata", "platform", "runbook.json"))
		require.NoError(t, err)
		assert.Contains(t, string(metadata), `"slug": "platform/runbook"`)
		assert.Contains(t, string(metadata), `"status": "verified"`)

		image, err := os.ReadFile(filepath.Join(output, "media", "7", "diagram.png"))
		require.NoError(t, err)
		assert.Equal(t, []byte("png"), image)

		attachment, err := os.ReadFile(filepath.Join(output, "attachments", "9", "notes.txt"))
		require.NoError(t, err)
		assert.Equal(t, []byte("notes"), attachment)

		manifest, err := os.ReadFile(filepath.Join(output, "manifest.json"))
		require.NoError(t, err)
		assert.Contains(t, string(manifest), `"format": 1`)
	})

	t.Run("replaces stale output atomically", func(t *testing.T) {
		t.Parallel()

		output := filepath.Join(t.TempDir(), "mirror")
		require.NoError(t, os.MkdirAll(output, 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(output, "stale"), []byte("old"), 0o644))

		require.NoError(t, Export(context.Background(), repositoryStub{}, output))
		_, err := os.Stat(filepath.Join(output, "stale"))
		assert.ErrorIs(t, err, os.ErrNotExist)
	})
}

func TestValidateOutputDir(t *testing.T) {
	t.Parallel()

	t.Run("rejects empty output", func(t *testing.T) {
		t.Parallel()
		assert.Error(t, validateOutputDir(""))
	})

	t.Run("accepts child output", func(t *testing.T) {
		t.Parallel()
		assert.NoError(t, validateOutputDir(filepath.Join(t.TempDir(), "mirror")))
	})
}
