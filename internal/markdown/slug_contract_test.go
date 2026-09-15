package markdown

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSlugContract(t *testing.T) {
	t.Parallel()
	data, err := os.ReadFile("../../test/contracts/slugs.json")
	require.NoError(t, err)

	type sharedFixture struct{ Name, Value, Slug string }
	var fixtures []sharedFixture
	require.NoError(t, json.Unmarshal(data, &fixtures))

	fixturesByName := make(map[string]sharedFixture, len(fixtures))
	for _, fixture := range fixtures {
		require.NotContains(t, fixturesByName, fixture.Name, "shared fixture names must be unique")
		fixturesByName[fixture.Name] = fixture
	}
	// Fail when the shared contract grows without a corresponding explicit subtest.
	require.Len(t, fixturesByName, 14)

	t.Run("empty", func(t *testing.T) {
		t.Parallel()

		fixture, ok := fixturesByName["empty"]
		require.True(t, ok, "shared fixture is missing")

		assert.Equal(t, fixture.Slug, Slug(fixture.Value))
	})

	t.Run("ordinary title", func(t *testing.T) {
		t.Parallel()

		fixture, ok := fixturesByName["ordinary title"]
		require.True(t, ok, "shared fixture is missing")

		assert.Equal(t, fixture.Slug, Slug(fixture.Value))
	})

	t.Run("hierarchy", func(t *testing.T) {
		t.Parallel()

		fixture, ok := fixturesByName["hierarchy"]
		require.True(t, ok, "shared fixture is missing")

		assert.Equal(t, fixture.Slug, Slug(fixture.Value))
	})

	t.Run("punctuation", func(t *testing.T) {
		t.Parallel()

		fixture, ok := fixturesByName["punctuation"]
		require.True(t, ok, "shared fixture is missing")

		assert.Equal(t, fixture.Slug, Slug(fixture.Value))
	})

	t.Run("punctuation at edges", func(t *testing.T) {
		t.Parallel()

		fixture, ok := fixturesByName["punctuation at edges"]
		require.True(t, ok, "shared fixture is missing")

		assert.Equal(t, fixture.Slug, Slug(fixture.Value))
	})

	t.Run("preserved separators", func(t *testing.T) {
		t.Parallel()

		fixture, ok := fixturesByName["preserved separators"]
		require.True(t, ok, "shared fixture is missing")

		assert.Equal(t, fixture.Slug, Slug(fixture.Value))
	})

	t.Run("non-ASCII separators", func(t *testing.T) {
		t.Parallel()

		fixture, ok := fixturesByName["non-ASCII separators"]
		require.True(t, ok, "shared fixture is missing")

		assert.Equal(t, fixture.Slug, Slug(fixture.Value))
	})

	t.Run("dotted capital I", func(t *testing.T) {
		t.Parallel()

		fixture, ok := fixturesByName["dotted capital I"]
		require.True(t, ok, "shared fixture is missing")

		assert.Equal(t, fixture.Slug, Slug(fixture.Value))
	})

	t.Run("dotted capital I after ASCII", func(t *testing.T) {
		t.Parallel()

		fixture, ok := fixturesByName["dotted capital I after ASCII"]
		require.True(t, ok, "shared fixture is missing")

		assert.Equal(t, fixture.Slug, Slug(fixture.Value))
	})

	t.Run("Kelvin sign", func(t *testing.T) {
		t.Parallel()

		fixture, ok := fixturesByName["Kelvin sign"]
		require.True(t, ok, "shared fixture is missing")

		assert.Equal(t, fixture.Slug, Slug(fixture.Value))
	})

	t.Run("emoji separator", func(t *testing.T) {
		t.Parallel()

		fixture, ok := fixturesByName["emoji separator"]
		require.True(t, ok, "shared fixture is missing")

		assert.Equal(t, fixture.Slug, Slug(fixture.Value))
	})

	t.Run("no transliteration", func(t *testing.T) {
		t.Parallel()

		fixture, ok := fixturesByName["no transliteration"]
		require.True(t, ok, "shared fixture is missing")

		assert.Equal(t, fixture.Slug, Slug(fixture.Value))
	})

	t.Run("slash spacing", func(t *testing.T) {
		t.Parallel()

		fixture, ok := fixturesByName["slash spacing"]
		require.True(t, ok, "shared fixture is missing")

		assert.Equal(t, fixture.Slug, Slug(fixture.Value))
	})

	t.Run("hyphen edges", func(t *testing.T) {
		t.Parallel()

		fixture, ok := fixturesByName["hyphen edges"]
		require.True(t, ok, "shared fixture is missing")

		assert.Equal(t, fixture.Slug, Slug(fixture.Value))
	})
}
