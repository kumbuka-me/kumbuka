package main

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParsePluginLock(t *testing.T) {
	t.Run("keeps package entries", func(t *testing.T) {
		entries, err := parsePluginLock([]byte("# bundled plugins\n\nalerts=1.2.3\ncalendar=2.0.0\n"))

		require.NoError(t, err)
		require.Equal(t, []pluginLockEntry{{name: "alerts", version: "1.2.3"}, {name: "calendar", version: "2.0.0"}}, entries)
	})

	t.Run("rejects entries without a version", func(t *testing.T) {
		_, err := parsePluginLock([]byte("alerts="))

		require.ErrorContains(t, err, `invalid lock entry "alerts="`)
	})

	t.Run("rejects entries without a separator", func(t *testing.T) {
		_, err := parsePluginLock([]byte("alerts"))

		require.ErrorContains(t, err, `invalid lock entry "alerts"`)
	})
}
