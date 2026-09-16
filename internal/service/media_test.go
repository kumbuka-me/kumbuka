package service

import (
	"context"
	"errors"
	"testing"

	"github.com/kumbuka-me/kumbuka/pkg/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mediaRepositoryStub struct {
	image          domain.Image
	imageErr       error
	deletedImageID int64
}

func (s *mediaRepositoryStub) AttachmentInfo(context.Context, int64) (domain.Attachment, error) {
	return domain.Attachment{}, nil
}

func (s *mediaRepositoryStub) AttachmentContent(context.Context, int64) (domain.AttachmentData, error) {
	return domain.AttachmentData{}, nil
}

func (s *mediaRepositoryStub) Attachments(context.Context) ([]domain.Attachment, error) {
	return nil, nil
}

func (s *mediaRepositoryStub) DeleteAttachment(context.Context, int64) error {
	return nil
}

func (s *mediaRepositoryStub) DeleteImage(_ context.Context, id int64) error {
	s.deletedImageID = id
	return nil
}

func (s *mediaRepositoryStub) ImageInfo(context.Context, int64) (domain.Image, error) {
	return s.image, s.imageErr
}

func (s *mediaRepositoryStub) ImageContent(context.Context, int64) (domain.ImageData, error) {
	return domain.ImageData{}, nil
}

func (s *mediaRepositoryStub) Images(context.Context) ([]domain.Image, error) {
	return nil, nil
}

func (s *mediaRepositoryStub) ImagesByUser(context.Context, int64) ([]domain.Image, error) {
	return nil, nil
}

func (s *mediaRepositoryStub) SearchImages(context.Context, string, int, int) ([]domain.Image, error) {
	return nil, nil
}

func (s *mediaRepositoryStub) SearchImagesByUser(context.Context, int64, string, int, int) ([]domain.Image, error) {
	return nil, nil
}

func (s *mediaRepositoryStub) SaveAttachment(
	context.Context,
	string,
	string,
	[]byte,
	int64,
) (domain.Attachment, error) {
	return domain.Attachment{}, nil
}

func (s *mediaRepositoryStub) SaveImage(
	context.Context,
	string,
	string,
	[]byte,
	int64,
) (domain.Image, error) {
	return domain.Image{}, nil
}

func TestDeleteImageEnforcesOwnership(t *testing.T) {
	t.Parallel()

	repository := &mediaRepositoryStub{image: domain.Image{ID: 7, UploadedBy: 2}}
	err := NewMedia(repository).DeleteImage(
		context.Background(),
		7,
		domain.User{ID: 1, Role: "editor"},
	)

	require.ErrorIs(t, err, ErrMediaForbidden)
	assert.Zero(t, repository.deletedImageID)
}

func TestDeleteImageRejectsReferencedOwnedMedia(t *testing.T) {
	t.Parallel()

	repository := &mediaRepositoryStub{image: domain.Image{
		ID:         7,
		UploadedBy: 1,
		UsageCount: 3,
	}}
	err := NewMedia(repository).DeleteImage(
		context.Background(),
		7,
		domain.User{ID: 1, Role: "editor"},
	)

	inUse, ok := errors.AsType[*MediaInUseError](err)

	require.True(t, ok)
	assert.Equal(t, int64(3), inUse.References)
	assert.Zero(t, repository.deletedImageID)
}

func TestDeleteImageAllowsAdministratorOverride(t *testing.T) {
	t.Parallel()

	repository := &mediaRepositoryStub{image: domain.Image{
		ID:         7,
		UploadedBy: 2,
		UsageCount: 3,
	}}
	err := NewMedia(repository).DeleteImage(
		context.Background(),
		7,
		domain.User{ID: 1, Role: "admin"},
	)

	require.NoError(t, err)
	assert.Equal(t, int64(7), repository.deletedImageID)
}

func TestSanitizeFilenameCharacterRuns(t *testing.T) {
	t.Run("collapses invalid runs", func(t *testing.T) {
		assert.Equal(t, "my-diagram.png", sanitizeFilename("my   diagram.png", "fallback"))
	})
	t.Run("collapses Unicode runs", func(t *testing.T) {
		assert.Equal(t, "a-b.png", sanitizeFilename("aé💡b.png", "fallback"))
	})
	t.Run("preserves hyphens", func(t *testing.T) {
		assert.Equal(t, "a--b.png", sanitizeFilename("a--b.png", "fallback"))
	})
	t.Run("separates invalid runs around hyphens", func(t *testing.T) {
		assert.Equal(t, "a---b.png", sanitizeFilename("a - b.png", "fallback"))
	})
	t.Run("uses basename", func(t *testing.T) {
		assert.Equal(t, "image.png", sanitizeFilename("  ../some/path/image.png  ", "fallback"))
	})
	t.Run("trims surrounding punctuation", func(t *testing.T) {
		assert.Equal(t, "hello_world", sanitizeFilename("._-hello_world-_.", "fallback"))
	})
	t.Run("uses fallback for invalid name", func(t *testing.T) {
		assert.Equal(t, "fallback", sanitizeFilename("💡...", "fallback"))
	})
	t.Run("uses fallback for empty name", func(t *testing.T) {
		assert.Equal(t, "fallback", sanitizeFilename("", "fallback"))
	})
	t.Run("preserves safe characters", func(t *testing.T) {
		assert.Equal(t, "Mixed.Case_123.txt", sanitizeFilename("Mixed.Case_123.txt", "fallback"))
	})
}
