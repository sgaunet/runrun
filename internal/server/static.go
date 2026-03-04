package server

import (
	"net/http"

	"github.com/sgaunet/runrun/internal/assets"
)

// serveStaticFiles sets up static file serving from embedded filesystem with cache headers
func (s *Server) serveStaticFiles() http.Handler {
	return assets.NewStaticFileServer()
}
