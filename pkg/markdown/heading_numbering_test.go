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

func TestRenumberHeadingsPreservesCodeAndLineEndings(t *testing.T) {
	source := "    # indented\r\n\t# tab\r\n~~~\r\n~~~ text\r\n# fenced\r\n~~~~\r\n# 2026 report\r\n"
	want := "    # indented\r\n\t# tab\r\n~~~\r\n~~~ text\r\n# fenced\r\n~~~~\r\n# 1. 2026 report\r\n"
	assert.Equal(t, want, RenumberHeadings(source))
}

func TestRomanHeadingNumbersSurviveSavingAgain(t *testing.T) {
	source := "# One\n## Two\n# Three\n# Four\n# Five"
	for format, want := range map[string]string{
		"roman":       "# I. One\n## I.I Two\n# II. Three\n# III. Four\n# IV. Five",
		"roman-lower": "# i. One\n## i.i Two\n# ii. Three\n# iii. Four\n# iv. Five",
	} {
		assert.Equal(t, want, RenumberHeadingsWithFormat(source, format))
		assert.Equal(t, want, RenumberHeadingsWithFormat(want, format))
	}
}
