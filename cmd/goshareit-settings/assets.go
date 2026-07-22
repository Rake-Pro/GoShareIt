//go:build darwin || windows

package main

import (
	"embed"
	"io/fs"
)

//go:embed all:frontend
var frontendFS embed.FS

// assets returns the frontend rooted at index.html.
func assets() fs.FS {
	sub, err := fs.Sub(frontendFS, "frontend")
	if err != nil {
		panic(err) // embed layout is fixed at compile time
	}
	return sub
}
