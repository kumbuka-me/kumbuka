package web

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAssets(t *testing.T) {
	t.Parallel()

	for _, name := range []string{
		"favicon.svg",
		"favicon-16x16.png",
		"favicon-32x32.png",
		"apple-touch-icon.png",
		"kumbuka.svg",
	} {
		file, err := Assets.Open(name)
		require.NoError(t, err, name)
		require.NoError(t, file.Close(), name)
	}
}
