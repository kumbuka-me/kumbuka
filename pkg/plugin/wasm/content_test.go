package wasm

import (
	"testing"

	"github.com/kumbuka-me/kumbuka/pkg/plugin"
	"github.com/kumbuka-me/sdk/pluginpackage"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExpandResourceSourceSkipsFencedCode(t *testing.T) {
	t.Parallel()

	module := resourceSubstitutionModule{module: pluginpackage.Module{Prefix: "var", ValueField: "value"}}
	records := map[string]plugin.ResourceRecord{
		"name": {Key: "name", Values: map[string]string{"value": "expanded"}},
	}

	prepared, used, err := module.expandResourceSource(plugin.Context{}, "before {{var:name}}\n```\n{{var:name}}\n```\nafter {{var:name}}", records, "token")

	require.NoError(t, err)
	assert.Equal(t, "before token0end\n```\n{{var:name}}\n```\nafter token0end", prepared.Markdown)
	assert.Equal(t, map[string]int{"name": 0}, used)
	assert.Len(t, prepared.Replacements, 1)
	assert.Equal(t, "expanded", prepared.Replacements[0].Value)
}

func TestValidateUsedExportOverridesRejectsUnusedKey(t *testing.T) {
	t.Parallel()

	err := validateUsedExportOverrides("example", "values", map[string]string{"unused": "value"}, map[string]int{"used": 0})

	var parameter *plugin.ParameterError
	require.ErrorAs(t, err, &parameter)
	assert.Equal(t, "example", parameter.PluginID)
	assert.Equal(t, "values", parameter.ModuleID)
	assert.Equal(t, "unused", parameter.Key)
}
