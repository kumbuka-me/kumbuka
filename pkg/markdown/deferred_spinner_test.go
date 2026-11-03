package markdown

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDeferredMacroPlaceholderUsesBoatSpinner(t *testing.T) {
	t.Parallel()

	html := deferredMacroPlaceholder(macroInvocation{
		owner:  "me.kumbuka.example",
		module: "remote-content",
	}, 2, "123", "en")

	assert.Contains(t, html, `class="kumbuka-boat-spinner kumbuka-boat-spinner--small"`)
	assert.Contains(t, html, `data-kumbuka-deferred-index="2"`)
	assert.Contains(t, html, `Loading external content…`)
	assert.NotContains(t, html, "kumbuka-deferred-spinner")
}
