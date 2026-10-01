package markdown

import (
	"testing"

	"github.com/kumbuka-me/kumbuka/pkg/domain"
	"github.com/stretchr/testify/assert"
)

func TestWikiLinkResolverUsesStablePageIdentity(t *testing.T) {
	resolve := WikiLinkResolver([]domain.PageLink{{
		TargetSlug:   "old-install-location",
		TargetID:     123,
		ResolvedSlug: "getting-started/install",
		Exists:       true,
	}})

	assert.Equal(t, "/p/123/getting-started/install", resolve("old-install-location"))
	assert.Equal(t, "/pages/missing", resolve("missing"))
}
