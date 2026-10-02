// Package plugins supplies embedded offline distribution candidates.
package plugins

import (
	"context"
	"embed"
	"github.com/kumbuka-me/kumbuka/pkg/plugin"
	"io/fs"
)

//go:embed *.kumbukaplugin
var Packages embed.FS

//go:generate go run ../scripts/generate-plugins -root ..

// Distribution provides cheap generated metadata and opens archives only on demand.
type Distribution struct{}

// Catalog returns metadata for all embedded first-party plugin packages.
func (Distribution) Catalog() []plugin.BuiltinPackage {
	return append([]plugin.BuiltinPackage(nil), catalog...)
}

// Package returns the embedded archive bytes for the requested first-party plugin.
func (Distribution) Package(_ context.Context, id string) ([]byte, error) {
	for _, item := range catalog {
		if item.ID == id {
			return Packages.ReadFile(item.ArchiveName)
		}
	}
	return nil, fs.ErrNotExist
}
