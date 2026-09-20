package endpoint

import (
	"io"
	"io/fs"
	"log/slog"
	"net/http"
	"testing"
	"testing/fstest"

	"github.com/kumbuka-me/kumbuka/internal/webview"
	"github.com/kumbuka-me/kumbuka/web"
	"github.com/stretchr/testify/require"
)

// testViewsLogger returns a logger that discards output from expected test failures.
func testViewsLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// testHandlerViews builds a production-shaped view renderer for handler tests.
func testHandlerViews(t testing.TB, runtime webview.RuntimeInfo) *webview.Views {
	t.Helper()
	return testHandlerViewsWithLogger(t, testViewsLogger(), runtime)
}

// testHandlerViewsWithLogger builds a production-shaped view renderer with the supplied logger.
func testHandlerViewsWithLogger(t testing.TB, logger *slog.Logger, runtime webview.RuntimeInfo) *webview.Views {
	t.Helper()

	views, err := webview.New(web.Assets, logger, "test", "test", nil, runtime)
	require.NoError(t, err)

	return views
}

// testHandlerViewsWithOverrides builds views from embedded assets after replacing selected files.
func testHandlerViewsWithOverrides(
	t testing.TB,
	logger *slog.Logger,
	runtime webview.RuntimeInfo,
	overrides map[string]string,
) *webview.Views {
	t.Helper()

	assets := make(fstest.MapFS)
	err := fs.WalkDir(web.Assets, ".", func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}

		data, err := fs.ReadFile(web.Assets, path)
		if err != nil {
			return err
		}
		assets[path] = &fstest.MapFile{Data: data}
		return nil
	})
	require.NoError(t, err)

	for path, source := range overrides {
		assets[path] = &fstest.MapFile{Data: []byte(source)}
	}

	views, err := webview.New(assets, logger, "test", "test", nil, runtime)
	require.NoError(t, err)

	return views
}

type viewDataServiceStub struct {
	load func(*http.Request, *webview.Views, string) (webview.Layout, error)
}

func (s viewDataServiceStub) Load(r *http.Request, views *webview.Views, title string) (webview.Layout, error) {
	return s.load(r, views, title)
}
