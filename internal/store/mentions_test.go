package store

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestMentionedUsernames(t *testing.T) {
	t.Run("no mentions", func(t *testing.T) {
		assert.Equal(t, []string(nil), mentionedUsernames("No mentions"))
	})
	t.Run("deduplicates ignoring case", func(t *testing.T) {
		assert.Equal(t, []string{"alice", "bob"}, mentionedUsernames("@Alice, @BOB and @alice"))
	})
	t.Run("ignores embedded at signs", func(t *testing.T) {
		assert.Equal(t, []string(nil), mentionedUsernames("mail@example.com prefix@user _@hidden 9@hidden"))
	})
	t.Run("accepts username punctuation", func(t *testing.T) {
		assert.Equal(t, []string{"a.b-c_d", "next"}, mentionedUsernames("(@a.b-c_d)\n@next"))
	})
	t.Run("ignores empty and unsupported names", func(t *testing.T) {
		assert.Equal(t, []string(nil), mentionedUsernames("@ @! @é"))
	})
	t.Run("accepts nonword boundaries", func(t *testing.T) {
		assert.Equal(t, []string{"alice", "bob"}, mentionedUsernames("é@alice @@bob"))
	})
	t.Run("does not reuse consumed boundaries", func(t *testing.T) {
		assert.Equal(t, []string{"a.", "c-"}, mentionedUsernames("@a.@b @c-@d"))
	})
	t.Run("accepts adjacent mentions", func(t *testing.T) {
		assert.Equal(t, []string{"one", "two"}, mentionedUsernames("@one,@two"))
	})
}
