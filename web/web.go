package web

import (
	"embed"
	"io/fs"
)

//go:embed dist
var embedded embed.FS
var Assets = mustSub(embedded, "dist")

// mustSub returns an embedded filesystem rooted at dir or panics on configuration errors.
func mustSub(source fs.FS, dir string) fs.FS {
	sub, err := fs.Sub(source, dir)
	if err != nil {
		panic(err)
	}

	return sub
}
