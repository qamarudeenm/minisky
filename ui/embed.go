package ui

import (
	"embed"
	"io"
	"io/fs"
	"net/http"
	"strings"
)

//go:embed dist/*
var Assets embed.FS

// Handler returns an HTTP FileServer with SPA fallback support. 
// It resolves the compiled Vite React Single-Page Application (SPA) by serving index.html
// for any non-existent paths (client-side routes).
func Handler() http.Handler {
	fsys, err := fs.Sub(Assets, "dist")
	if err != nil {
		panic(err)
	}

	staticHandler := http.FileServer(http.FS(fsys))

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/")
		
		// If requesting a specific file that exists, serve it
		if path != "" {
			_, err := fsys.Open(path)
			if err == nil {
				staticHandler.ServeHTTP(w, r)
				return
			}
		}

		// Fallback: Serve index.html for SPA routing
		file, err := fsys.Open("index.html")
		if err != nil {
			http.Error(w, "Frontend build (index.html) missing", http.StatusNotFound)
			return
		}
		defer file.Close()

		// Read and serve index.html
		content, err := io.ReadAll(file)
		if err != nil {
			http.Error(w, "Failed to read index.html", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		w.Write(content)
	})
}
