package ui

import (
	"embed"
	"io/fs"
	"net/http"
)

//go:embed dist/*
var Assets embed.FS

// Handler returns an HTTP FileServer natively resolving the compiled Vite React Single-Page Application (SPA)
func Handler() http.Handler {
	// Focus the FileServer into the 'dist' directory to drop the wrapper folder
	fsys, err := fs.Sub(Assets, "dist")
	if err != nil {
		panic(err)
	}
	return http.FileServer(http.FS(fsys))
}
