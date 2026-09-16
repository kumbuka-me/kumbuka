package pluginusage

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIndexHas(t *testing.T) {
	t.Parallel()
	index := Index{Modules: []Module{{PluginID: "io.example", ModuleID: "feature"}}}
	assert.True(t, index.Has("io.example", "feature"))
	assert.False(t, index.Has("io.example", "other"))
}
