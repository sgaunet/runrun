package server

import (
	"embed"
	"io/fs"
	"net/http"
)

//go:embed static/*
var staticFiles embed.FS

// serveStaticFiles sets up static file serving from embedded filesystem
func (s *Server) serveStaticFiles() http.Handler {
	// Get the static subdirectory from embedded filesystem
	staticFS, err := fs.Sub(staticFiles, "static")
	if err != nil {
		// Fallback to regular filesystem if embed fails
		return http.FileServer(http.Dir("../../static"))
	}

	return http.FileServer(http.FS(staticFS))
}
