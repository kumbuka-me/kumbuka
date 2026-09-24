package endpoint

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"

	apppages "github.com/kumbuka-me/kumbuka/internal/application/pages"
	"github.com/kumbuka-me/kumbuka/internal/application/portablearchive"
	"github.com/kumbuka-me/kumbuka/internal/portable"
	"github.com/kumbuka-me/kumbuka/pkg/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// portableCatalogStub returns pages from an in-memory slug map.
type portableCatalogStub map[string]domain.Page

// GetPage returns one configured page.
func (s portableCatalogStub) GetPage(_ context.Context, slug string) (domain.Page, error) {
	page, ok := s[slug]
	if !ok {
		return domain.Page{}, domain.ErrNotFound
	}
	return page, nil
}

// portableExportMediaStub returns fixed image and attachment payloads.
type portableExportMediaStub struct{}

// ImageContent returns one deterministic PNG-like payload.
func (portableExportMediaStub) ImageContent(_ context.Context, id int64) (domain.ImageData, error) {
	if id != 12 {
		return domain.ImageData{}, domain.ErrNotFound
	}
	return domain.ImageData{Filename: "diagram.png", ContentType: "image/png", Data: []byte("image")}, nil
}

// AttachmentContent returns one deterministic attachment payload.
func (portableExportMediaStub) AttachmentContent(_ context.Context, id int64) (domain.AttachmentData, error) {
	if id != 7 {
		return domain.AttachmentData{}, domain.ErrNotFound
	}
	return domain.AttachmentData{
		Attachment: domain.Attachment{ID: 7, Filename: "runbook.pdf", ContentType: "application/pdf"},
		Data:       []byte("attachment"),
	}, nil
}

// TestWritePortableExportArchiveIncludesMetadataAndResources supports portable archive regression coverage.
func TestWritePortableExportArchiveIncludesMetadataAndResources(t *testing.T) {
	t.Parallel()

	catalog := portableCatalogStub{
		"platform/runbook": {
			Slug:               "platform/runbook",
			Title:              "Runbook",
			Language:           "en",
			Markdown:           "![Diagram](/media/12/diagram.png)\n\n[Manual](/attachments/7/runbook.pdf)\n",
			Tags:               []string{"ops", "platform"},
			Groups:             []domain.Group{{ID: 3, Name: "Platform"}},
			Status:             "verified",
			OwnerGroupID:       3,
			OwnerGroup:         "Platform",
			ReviewIntervalDays: 180,
			Properties:         []domain.PageProperty{{Key: "tier", Value: "critical"}},
		},
	}

	var output bytes.Buffer
	err := portablearchive.WritePortable(
		context.Background(),
		catalog,
		portableExportMediaStub{},
		&output,
		[]string{"platform/runbook"},
	)
	require.NoError(t, err)

	files := readTestZip(t, output.Bytes())
	assert.Contains(t, files, portable.ManifestPath)
	assert.Equal(t, "image", string(files["media/12/diagram.png"]))
	assert.Equal(t, "attachment", string(files["attachments/7/runbook.pdf"]))
	assert.Contains(t, string(files["pages/platform/runbook.md"]), "../../media/12/diagram.png")
	assert.Contains(t, string(files["pages/platform/runbook.md"]), "../../attachments/7/runbook.pdf")

	var manifest portable.Manifest
	require.NoError(t, json.Unmarshal(files[portable.ManifestPath], &manifest))
	assert.Equal(t, portable.Format, manifest.Format)
	assert.Equal(t, portable.Version, manifest.Version)
	require.Len(t, manifest.Pages, 1)
	require.Len(t, manifest.Media, 1)
	require.Len(t, manifest.Attachments, 1)

	var metadata portable.PageMetadata
	require.NoError(t, json.Unmarshal(files["metadata/platform/runbook.json"], &metadata))
	assert.Equal(t, "Runbook", metadata.Title)
	assert.Equal(t, []string{"Platform"}, metadata.Groups)
	assert.Equal(t, "Platform", metadata.OwnerGroup)
	assert.Equal(t, map[string]string{"tier": "critical"}, metadata.Properties)
}

// TestImportPagesWithPortableArchiveAutoDetectsKumbukaZip supports portable archive regression coverage.
func TestImportPagesWithPortableArchiveAutoDetectsKumbukaZip(t *testing.T) {
	t.Parallel()

	manifest := portable.NewManifest()
	manifest.Pages = []portable.PageEntry{{
		Slug: "adfadf", Markdown: "pages/adfadf.md", Metadata: "metadata/adfadf.json",
	}}
	archive := testPortableArchive(t, manifest, map[string][]byte{
		"pages/adfadf.md": []byte("Body without a level-one heading.\n"),
		"metadata/adfadf.json": mustJSON(t, portable.PageMetadata{
			Slug: "adfadf", Title: "ADFADF", Status: "verified",
		}),
	})

	var body bytes.Buffer
	multipartWriter := multipart.NewWriter(&body)
	require.NoError(t, multipartWriter.WriteField("format", "markdown"))
	file, err := multipartWriter.CreateFormFile("files", "kumbuka-export.zip")
	require.NoError(t, err)
	_, err = file.Write(archive)
	require.NoError(t, err)
	require.NoError(t, multipartWriter.Close())

	request := httptest.NewRequest(http.MethodPost, "/admin/import", &body)
	request.Header.Set("Content-Type", multipartWriter.FormDataContentType())
	response := httptest.NewRecorder()
	pages := &portableRestorePagesStub{}

	ImportPagesWithPortableArchive(
		pages,
		&portableRestoreMediaStub{},
		&portableRestoreGroupsStub{},
		slog.Default(),
	).ServeHTTP(response, request)

	assert.Equal(t, http.StatusSeeOther, response.Code)
	require.Len(t, pages.Pages, 1)
	assert.Equal(t, "ADFADF", pages.Pages[0].Title)
	assert.Equal(t, "Body without a level-one heading.\n", pages.Pages[0].Markdown)
}

// portableRestorePagesStub records pages passed to the portable service import.
type portableRestorePagesStub struct {
	// Pages contains the reconstructed page mutations.
	Pages []apppages.PortableImportedPage
}

// Import satisfies the legacy page import contract used by the combined handler.
func (*portableRestorePagesStub) Import(context.Context, []apppages.ImportedPage, string, domain.User) (int, error) {
	return 0, nil
}

// ImportPortable records portable page mutations.
func (s *portableRestorePagesStub) ImportPortable(
	_ context.Context,
	pages []apppages.PortableImportedPage,
	_ domain.User,
) (int, error) {
	s.Pages = pages
	return len(pages), nil
}

// ImportPortablePages records the pages prepared inside the import transaction.
func (s *portableRestorePagesStub) ImportPortablePages(ctx context.Context, pages []apppages.PortableImportedPage, actor domain.User) (int, error) {
	return s.ImportPortable(ctx, pages, actor)
}

// RunPortableImport executes the test transaction callback once.
func (*portableRestorePagesStub) RunPortableImport(ctx context.Context, _ domain.User, run func(context.Context) (int, error)) (int, error) {
	return run(ctx)
}

// portableRestoreMediaStub records recreated resources and returns target identifiers.
type portableRestoreMediaStub struct {
	// Images contains uploaded image filenames.
	Images []string
	// Attachments contains uploaded attachment filenames.
	Attachments []string
}

// UploadImage records one image upload and returns its target URL identity.
func (s *portableRestoreMediaStub) UploadImage(
	_ context.Context,
	filename string,
	_ []byte,
	_ domain.User,
) (domain.Image, error) {
	s.Images = append(s.Images, filename)
	return domain.Image{ID: 101, Filename: filename}, nil
}

// UploadAttachment records one attachment upload and returns its target URL identity.
func (s *portableRestoreMediaStub) UploadAttachment(
	_ context.Context,
	filename string,
	_ []byte,
	_ domain.User,
) (domain.Attachment, error) {
	s.Attachments = append(s.Attachments, filename)
	return domain.Attachment{ID: 202, Filename: filename}, nil
}

// portableRestoreGroupsStub resolves existing groups and creates missing ones.
type portableRestoreGroupsStub struct {
	// GroupsValue contains existing target groups.
	GroupsValue []domain.Group
	// Created contains group names created during restore.
	Created []string
}

// Groups returns configured existing groups.
func (s *portableRestoreGroupsStub) Groups(context.Context) ([]domain.Group, error) {
	return s.GroupsValue, nil
}

// CreateGroup records and returns a newly created group.
func (s *portableRestoreGroupsStub) CreateGroup(_ context.Context, name string) (domain.Group, error) {
	s.Created = append(s.Created, name)
	return domain.Group{ID: int64(10 + len(s.Created)), Name: name}, nil
}

// TestRestorePortableArchiveRewritesResourcesAndMapsGroups supports portable archive regression coverage.
func TestRestorePortableArchiveRewritesResourcesAndMapsGroups(t *testing.T) {
	t.Parallel()

	pages := &portableRestorePagesStub{}
	media := &portableRestoreMediaStub{}
	groups := &portableRestoreGroupsStub{GroupsValue: []domain.Group{{ID: 4, Name: "Platform"}}}
	archive := portable.Archive{
		Manifest: portable.Manifest{
			Format:  portable.Format,
			Version: portable.Version,
			Media: []portable.ResourceEntry{{
				Path: "media/12/diagram.png", Filename: "diagram.png",
			}},
			Attachments: []portable.ResourceEntry{{
				Path: "attachments/7/runbook.pdf", Filename: "runbook.pdf",
			}},
		},
		Resources: map[string][]byte{
			"media/12/diagram.png":      []byte("image"),
			"attachments/7/runbook.pdf": []byte("attachment"),
		},
		Pages: []portable.Page{{
			Entry: portable.PageEntry{Slug: "platform/runbook", Markdown: "pages/platform/runbook.md"},
			Metadata: portable.PageMetadata{
				Slug: "platform/runbook", Title: "Runbook", Status: "verified",
				Groups: []string{"Platform", "Ops"}, OwnerGroup: "Platform",
			},
			Markdown: "![Diagram](../../media/12/diagram.png)\n[Manual](../../attachments/7/runbook.pdf)",
		}},
	}

	count, err := portablearchive.Restore(
		context.Background(), archive, pages, media, groups, domain.User{ID: 1},
	)

	require.NoError(t, err)
	assert.Equal(t, 1, count)
	assert.Equal(t, []string{"diagram.png"}, media.Images)
	assert.Equal(t, []string{"runbook.pdf"}, media.Attachments)
	assert.Equal(t, []string{"Ops"}, groups.Created)
	require.Len(t, pages.Pages, 1)
	assert.Contains(t, pages.Pages[0].Markdown, "/media/101/diagram.png")
	assert.Contains(t, pages.Pages[0].Markdown, "/attachments/202/runbook.pdf")
	assert.Equal(t, []int64{4, 11}, pages.Pages[0].GroupIDs)
	assert.EqualValues(t, 4, pages.Pages[0].OwnerGroupID)
}

// readTestZip returns every non-directory entry from a ZIP payload.
func readTestZip(t *testing.T, data []byte) map[string][]byte {
	t.Helper()
	reader, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	require.NoError(t, err)
	files := map[string][]byte{}
	for _, entry := range reader.File {
		if entry.FileInfo().IsDir() {
			continue
		}
		file, err := entry.Open()
		require.NoError(t, err)
		content, err := io.ReadAll(file)
		require.NoError(t, err)
		require.NoError(t, file.Close())
		files[entry.Name] = content
	}
	return files
}

// testPortableArchive builds one portable ZIP from a manifest and additional entries.
func testPortableArchive(t *testing.T, manifest portable.Manifest, files map[string][]byte) []byte {
	t.Helper()
	var output bytes.Buffer
	writer := zip.NewWriter(&output)
	require.NoError(t, portablearchive.WriteZIPJSON(writer, portable.ManifestPath, manifest))
	for name, data := range files {
		require.NoError(t, portablearchive.WriteZIPBytes(writer, name, data))
	}
	require.NoError(t, writer.Close())
	return output.Bytes()
}

// mustJSON encodes one test fixture as JSON.
func mustJSON(t *testing.T, value any) []byte {
	t.Helper()
	data, err := json.Marshal(value)
	require.NoError(t, err)
	return data
}
