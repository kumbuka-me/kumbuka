package mention

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestUsernames(t *testing.T) {
	t.Parallel()

	t.Run("returns distinct normalized mentions", func(t *testing.T) {
		t.Parallel()
		assert.Equal(t, []string{"alice", "bob"}, Usernames("@Alice, @BOB and @alice"))
	})
	t.Run("ignores embedded at signs", func(t *testing.T) {
		t.Parallel()
		assert.Equal(t, []string(nil), Usernames("mail@example.com prefix@user _@hidden 9@hidden"))
	})
	t.Run("accepts username punctuation", func(t *testing.T) {
		t.Parallel()
		assert.Equal(t, []string{"a.b-c_d", "next"}, Usernames("(@a.b-c_d)\n@next"))
	})
	t.Run("ignores empty and unsupported names", func(t *testing.T) {
		t.Parallel()
		assert.Equal(t, []string(nil), Usernames("@ @! @é"))
	})
	t.Run("accepts nonword boundaries", func(t *testing.T) {
		t.Parallel()
		assert.Equal(t, []string{"alice", "bob"}, Usernames("é@alice @@bob"))
	})
	t.Run("does not reuse consumed boundaries", func(t *testing.T) {
		t.Parallel()
		assert.Equal(t, []string{"a.", "c-"}, Usernames("@a.@b @c-@d"))
	})
	t.Run("accepts adjacent mentions", func(t *testing.T) {
		t.Parallel()
		assert.Equal(t, []string{"one", "two"}, Usernames("@one,@two"))
	})
	t.Run("ignores mentions in plugin declarations", func(t *testing.T) {
		t.Parallel()
		assert.Equal(t, []string{"bob"}, Usernames(`{{task assignee="@alice"}} Please review, @bob!`))
	})
	t.Run("ignores mentions after an unterminated plugin declaration", func(t *testing.T) {
		t.Parallel()
		assert.Equal(t, []string(nil), Usernames(`{{task assignee="@alice"`))
	})
}

func TestRangesPreserveSourceOffsets(t *testing.T) {
	t.Parallel()

	text := "Hello @Admin and @alice."
	assert.Equal(t, []Range{
		{Start: 6, End: 12, Username: "admin"},
		{Start: 17, End: 24, Username: "alice."},
	}, Ranges(text))
}
