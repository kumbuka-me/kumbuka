// Package plugins supplies bundled plugin distribution bytes, not a separate plugin runtime.
package plugins

import (
	"embed"
	"io/fs"
)

//go:embed *.kumbukaplugin
var Packages embed.FS

// Archives returns embedded bundled plugin archives in deterministic filename order.
func Archives() ([][]byte, error) {
	names, err := fs.Glob(Packages, "*.kumbukaplugin")
	if err != nil {
		return nil, err
	}
	archives := make([][]byte, 0, len(names))
	for _, name := range names {
		data, err := Packages.ReadFile(name)
		if err != nil {
			return nil, err
		}
		archives = append(archives, data)
	}
	return archives, nil
}
