package markdown

import "testing"

import "github.com/stretchr/testify/assert"

func TestRenumberHeadings(t *testing.T) {
	t.Parallel()

	source := "# Introduction\n\n## Install\n\n### Linux\n\n# Operations\n\n## Backup\n\n```markdown\n# Literal\n```\n"
	want := "# 1. Introduction\n\n## 1.1 Install\n\n### 1.1.1 Linux\n\n# 2. Operations\n\n## 2.1 Backup\n\n```markdown\n# Literal\n```\n"

	assert.Equal(t, want, RenumberHeadings(source))
	assert.Equal(t, want, RenumberHeadings(want))
	assert.True(t, HeadingNumberingEnabled(map[string]string{HeadingNumberingProperty: "true"}))
	assert.False(t, HeadingNumberingEnabled(nil))
}
