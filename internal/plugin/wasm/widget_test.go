package wasm

import (
	"testing"

	"github.com/kumbuka-me/sdk"
	"github.com/stretchr/testify/assert"
)

func TestValidWidgetAction(t *testing.T) {
	t.Parallel()

	valid := sdk.WidgetAction{ID: "all", Kind: "dialog", Label: "All revisions", URL: "/revisions/guide", Icon: "history-lucide"}
	assert.True(t, validWidgetAction(valid, "widget"))

	for _, action := range []sdk.WidgetAction{
		{ID: "all", Kind: "dialog", Label: "All revisions", URL: "https://example.test"},
		{ID: "all", Kind: "dialog", Label: "All revisions", URL: "//example.test"},
		{ID: "../bad", Kind: "dialog", Label: "All revisions", URL: "/revisions/guide"},
		{ID: "all", Kind: "script", Label: "All revisions", URL: "/revisions/guide"},
	} {
		assert.False(t, validWidgetAction(action, "widget"), action)
	}
	assert.False(t, validWidgetAction(valid, "postprocess"))
}
