package auth

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLocalPasswordSharedContract(t *testing.T) {
	t.Parallel()

	data, err := os.ReadFile("../../test/contracts/passwords.json")
	require.NoError(t, err)

	type sharedFixture struct{ Name, Password, Problem string }
	var fixtures []sharedFixture
	require.NoError(t, json.Unmarshal(data, &fixtures))

	fixturesByName := make(map[string]sharedFixture, len(fixtures))
	for _, fixture := range fixtures {
		require.NotContains(t, fixturesByName, fixture.Name, "shared fixture names must be unique")
		fixturesByName[fixture.Name] = fixture
	}

	// Fail when the shared contract grows without a corresponding explicit subtest.
	require.Len(t, fixturesByName, 10)

	t.Run("short", func(t *testing.T) {
		t.Parallel()

		fixture, ok := fixturesByName["short"]
		require.True(t, ok, "shared fixture is missing")

		assert.Equal(t, fixture.Problem, LocalPasswordProblem(fixture.Password))
	})

	t.Run("minimum", func(t *testing.T) {
		t.Parallel()

		fixture, ok := fixturesByName["minimum"]
		require.True(t, ok, "shared fixture is missing")

		assert.Equal(t, fixture.Problem, LocalPasswordProblem(fixture.Password))
	})

	t.Run("ascii-limit", func(t *testing.T) {
		t.Parallel()

		fixture, ok := fixturesByName["ascii-limit"]
		require.True(t, ok, "shared fixture is missing")

		assert.Equal(t, fixture.Problem, LocalPasswordProblem(fixture.Password))
	})

	t.Run("ascii-too-long", func(t *testing.T) {
		t.Parallel()

		fixture, ok := fixturesByName["ascii-too-long"]
		require.True(t, ok, "shared fixture is missing")

		assert.Equal(t, fixture.Problem, LocalPasswordProblem(fixture.Password))
	})

	t.Run("six-emoji", func(t *testing.T) {
		t.Parallel()

		fixture, ok := fixturesByName["six-emoji"]
		require.True(t, ok, "shared fixture is missing")

		assert.Equal(t, fixture.Problem, LocalPasswordProblem(fixture.Password))
	})

	t.Run("twelve-emoji", func(t *testing.T) {
		t.Parallel()

		fixture, ok := fixturesByName["twelve-emoji"]
		require.True(t, ok, "shared fixture is missing")

		assert.Equal(t, fixture.Problem, LocalPasswordProblem(fixture.Password))
	})

	t.Run("emoji-limit", func(t *testing.T) {
		t.Parallel()

		fixture, ok := fixturesByName["emoji-limit"]
		require.True(t, ok, "shared fixture is missing")

		assert.Equal(t, fixture.Problem, LocalPasswordProblem(fixture.Password))
	})

	t.Run("emoji-too-long", func(t *testing.T) {
		t.Parallel()

		fixture, ok := fixturesByName["emoji-too-long"]
		require.True(t, ok, "shared fixture is missing")

		assert.Equal(t, fixture.Problem, LocalPasswordProblem(fixture.Password))
	})

	t.Run("accent-limit", func(t *testing.T) {
		t.Parallel()

		fixture, ok := fixturesByName["accent-limit"]
		require.True(t, ok, "shared fixture is missing")

		assert.Equal(t, fixture.Problem, LocalPasswordProblem(fixture.Password))
	})

	t.Run("accent-too-long", func(t *testing.T) {
		t.Parallel()

		fixture, ok := fixturesByName["accent-too-long"]
		require.True(t, ok, "shared fixture is missing")

		assert.Equal(t, fixture.Problem, LocalPasswordProblem(fixture.Password))
	})
}

func TestLocalPasswordProblemRejectsInvalidUTF8(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "Use valid UTF-8 characters.", LocalPasswordProblem("valid-length-"+string([]byte{0xff})))
}
