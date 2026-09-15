package site

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestConfigFileValuesRejectEmptySettings(t *testing.T) {
	t.Parallel()

	t.Run("site name", func(t *testing.T) {
		t.Parallel()

		config := defaultConfig()
		config.SiteName = ""

		assert.ErrorContains(t, validateConfigFileValues(config), "site_name must not be empty")
	})

	t.Run("source directory", func(t *testing.T) {
		t.Parallel()

		config := defaultConfig()
		config.SourceDir = ""

		assert.ErrorContains(t, validateConfigFileValues(config), "source_dir must not be empty")
	})

	t.Run("output directory", func(t *testing.T) {
		t.Parallel()

		config := defaultConfig()
		config.OutputDir = ""

		assert.ErrorContains(t, validateConfigFileValues(config), "output_dir must not be empty")
	})

	t.Run("theme", func(t *testing.T) {
		t.Parallel()

		config := defaultConfig()
		config.Theme = ""

		assert.ErrorContains(t, validateConfigFileValues(config), "theme must not be empty")
	})

	t.Run("language", func(t *testing.T) {
		t.Parallel()

		config := defaultConfig()
		config.Language = ""

		assert.ErrorContains(t, validateConfigFileValues(config), "language must not be empty")
	})
}
