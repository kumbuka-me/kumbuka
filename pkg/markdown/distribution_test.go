package markdown

import (
	"github.com/kumbuka-me/kumbuka/pkg/plugin"
	"github.com/stretchr/testify/require"
	"testing"
)

func testDistribution(t testing.TB, archives [][]byte) plugin.Distribution {
	t.Helper()
	d, err := plugin.NewArchiveDistribution(archives)
	require.NoError(t, err)
	return d
}
