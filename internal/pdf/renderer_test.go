package pdf

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDocumentEscapesThePageTitle(t *testing.T) {
	t.Parallel()

	result := Document(`Runbooks <production>`, "de-CH", `<p>Content</p>`)

	assert.Contains(t, result, `<html lang="de-CH">`)
	assert.Contains(t, result, `<title>Runbooks &lt;production&gt;</title>`)
	assert.Contains(t, result, `<h1 class="document-title">Runbooks &lt;production&gt;</h1>`)
}

func TestDocumentExpandsInteractiveMarkdown(t *testing.T) {
	t.Parallel()

	result := Document(
		"Runbook",
		"en",
		`<details class="markdown-details"><summary>Steps</summary><div class="markdown-tab-panel markdown-tab-panel-hidden">Deploy</div></details>`,
	)

	assert.NotContains(t, result, "<details")
	assert.NotContains(t, result, "<summary>")
	assert.NotContains(t, result, `class="markdown-tab-panel markdown-tab-panel-hidden"`)
	assert.Contains(t, result, `<div class="markdown-details-summary">Steps</div>`)
}

func TestRenderPostsToExactConfiguredEndpoint(t *testing.T) {
	t.Parallel()

	payload := "%PDF-1.7\nfixture\n%%EOF\n"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Equal(t, "/custom/pdf/render?profile=wiki", r.URL.RequestURI())
		assert.Equal(t, "text/html; charset=utf-8", r.Header.Get("Content-Type"))
		assert.Equal(t, "application/pdf", r.Header.Get("Accept"))
		assert.Equal(t, "Bearer secret-token", r.Header.Get("Authorization"))

		body, err := io.ReadAll(r.Body)

		if !assert.NoError(t, err) {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		assert.Contains(t, string(body), `<html lang="de-CH">`)
		assert.Contains(t, string(body), `Title &lt;test&gt;`)
		assert.Contains(t, string(body), `data:image/png;base64,AAAA`)
		w.Header().Set("Content-Type", "application/pdf")

		_, _ = io.WriteString(w, payload)
	}))
	defer server.Close()

	file, cleanup, err := Render(
		context.Background(),
		server.URL+"/custom/pdf/render?profile=wiki",
		"Title <test>",
		"de-CH",
		`<img src="data:image/png;base64,AAAA">`,
		http.Header{"Authorization": {"Bearer secret-token"}},
	)

	require.NoError(t, err)
	defer cleanup()

	body, err := io.ReadAll(file)

	require.NoError(t, err)
	assert.Equal(t, payload, string(body))

	name := file.Name()

	cleanup()

	_, err = os.Stat(name)

	assert.True(t, os.IsNotExist(err))
}

func TestRenderRejectsBadResponses(t *testing.T) {
	t.Parallel()
	t.Run("upstream error", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, "/render", r.URL.Path, "redirect must not be followed")

			w.Header().Set("Location", "/unexpected")
			w.Header().Set("Content-Type", "text/plain")

			w.WriteHeader(500)

			_, _ = io.WriteString(w, "internal error")
		}))
		defer server.Close()

		file, cleanup, err := Render(context.Background(), server.URL+"/render", "Title", "en", "<p>test</p>", nil)

		cleanup()
		require.Error(t, err)
		assert.Nil(t, file)
	})

	t.Run("redirect", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, "/render", r.URL.Path, "redirect must not be followed")

			w.Header().Set("Location", "/unexpected")
			w.Header().Set("Content-Type", "application/pdf")

			w.WriteHeader(307)

			_, _ = io.WriteString(w, "%PDF-1.7")
		}))
		defer server.Close()

		file, cleanup, err := Render(context.Background(), server.URL+"/render", "Title", "en", "<p>test</p>", nil)

		cleanup()
		require.Error(t, err)
		assert.Nil(t, file)
	})

	t.Run("HTML response", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, "/render", r.URL.Path, "redirect must not be followed")

			w.Header().Set("Location", "/unexpected")
			w.Header().Set("Content-Type", "text/html")

			w.WriteHeader(200)

			_, _ = io.WriteString(w, "<html>error</html>")
		}))
		defer server.Close()

		file, cleanup, err := Render(context.Background(), server.URL+"/render", "Title", "en", "<p>test</p>", nil)

		cleanup()
		require.Error(t, err)
		assert.Nil(t, file)
	})

	t.Run("invalid bytes", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, "/render", r.URL.Path, "redirect must not be followed")

			w.Header().Set("Location", "/unexpected")
			w.Header().Set("Content-Type", "application/pdf")

			w.WriteHeader(200)

			_, _ = io.WriteString(w, "not a pdf")
		}))
		defer server.Close()

		file, cleanup, err := Render(context.Background(), server.URL+"/render", "Title", "en", "<p>test</p>", nil)

		cleanup()
		require.Error(t, err)
		assert.Nil(t, file)
	})

	t.Run("empty response", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, "/render", r.URL.Path, "redirect must not be followed")

			w.Header().Set("Location", "/unexpected")
			w.Header().Set("Content-Type", "application/pdf")

			w.WriteHeader(200)

			_, _ = io.WriteString(w, "")
		}))
		defer server.Close()

		file, cleanup, err := Render(context.Background(), server.URL+"/render", "Title", "en", "<p>test</p>", nil)

		cleanup()
		require.Error(t, err)
		assert.Nil(t, file)
	})

	t.Run("oversized response", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, "/render", r.URL.Path, "redirect must not be followed")

			w.Header().Set("Location", "/unexpected")
			w.Header().Set("Content-Type", "application/pdf")

			if strconv.Itoa(maxPDFBytes+1) != "" {
				w.Header().Set("Content-Length", strconv.Itoa(maxPDFBytes+1))
			}

			w.WriteHeader(200)

			_, _ = io.WriteString(w, "%PDF-1.7")
		}))
		defer server.Close()

		file, cleanup, err := Render(context.Background(), server.URL+"/render", "Title", "en", "<p>test</p>", nil)

		cleanup()
		require.Error(t, err)
		assert.Nil(t, file)
	})

	t.Run("truncated response", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, "/render", r.URL.Path, "redirect must not be followed")

			w.Header().Set("Location", "/unexpected")
			w.Header().Set("Content-Type", "application/pdf")

			w.Header().Set("Content-Length", "100")

			w.WriteHeader(200)

			_, _ = io.WriteString(w, "%PDF-1.7")
		}))
		defer server.Close()

		file, cleanup, err := Render(context.Background(), server.URL+"/render", "Title", "en", "<p>test</p>", nil)

		cleanup()
		require.Error(t, err)
		assert.Nil(t, file)
	})
}

func TestRenderCanceledAndUnconfigured(t *testing.T) {
	t.Parallel()

	t.Run("unconfigured service", func(t *testing.T) {
		t.Parallel()

		_, cleanup, err := Render(context.Background(), "", "", "", "", nil)
		cleanup()
		assert.ErrorIs(t, err, ErrNotConfigured)
	})

	t.Run("canceled context", func(t *testing.T) {
		t.Parallel()

		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		_, cleanup, err := Render(ctx, "http://127.0.0.1:1/render", "", "", "", nil)
		cleanup()
		assert.ErrorIs(t, err, context.Canceled)
	})
}

func TestRenderBoundsUnknownLengthResponse(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/pdf")

		_, _ = io.WriteString(w, "%PDF-")

		w.(http.Flusher).Flush()

		chunk := strings.Repeat("x", 1<<20)

		for range 65 {
			if _, err := io.WriteString(w, chunk); err != nil {
				return
			}
		}
	}))
	defer server.Close()

	file, cleanup, err := Render(context.Background(), server.URL+"/render", "Title", "en", "<p>test</p>", nil)

	cleanup()
	require.ErrorContains(t, err, "64 MiB")
	assert.Nil(t, file)
}
