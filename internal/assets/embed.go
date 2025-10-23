package assets

import (
	"embed"
	"io/fs"
	"net/http"
	"strings"
	"time"
)

// Static assets embedded into the binary
//
//go:embed all:static
var staticFS embed.FS

// GetStaticFS returns the embedded filesystem for static assets
func GetStaticFS() (fs.FS, error) {
	// Return a sub-filesystem starting at "static" directory
	return fs.Sub(staticFS, "static")
}

// NewStaticFileServer creates an HTTP handler that serves static files
// from the embedded filesystem with proper cache headers
func NewStaticFileServer() http.Handler {
	staticFiles, err := GetStaticFS()
	if err != nil {
		panic(err)
	}

	fileServer := http.FileServer(http.FS(staticFiles))

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Set cache headers for static assets
		path := r.URL.Path

		// Cache CSS and JS for 1 hour (since we might update them frequently during development)
		// In production, you might want longer cache times with versioned filenames
		if strings.HasSuffix(path, ".css") || strings.HasSuffix(path, ".js") {
			w.Header().Set("Cache-Control", "public, max-age=3600")
		}

		// Cache images and fonts for 1 week
		if strings.HasSuffix(path, ".png") || strings.HasSuffix(path, ".jpg") ||
			strings.HasSuffix(path, ".jpeg") || strings.HasSuffix(path, ".gif") ||
			strings.HasSuffix(path, ".svg") || strings.HasSuffix(path, ".ico") ||
			strings.HasSuffix(path, ".woff") || strings.HasSuffix(path, ".woff2") ||
			strings.HasSuffix(path, ".ttf") || strings.HasSuffix(path, ".eot") {
			w.Header().Set("Cache-Control", "public, max-age=604800")
		}

		// Set content type headers
		if strings.HasSuffix(path, ".css") {
			w.Header().Set("Content-Type", "text/css; charset=utf-8")
		} else if strings.HasSuffix(path, ".js") {
			w.Header().Set("Content-Type", "application/javascript; charset=utf-8")
		}

		// Serve the file
		fileServer.ServeHTTP(w, r)
	})
}

// Asset represents metadata about an embedded asset
type Asset struct {
	Path         string
	Size         int64
	ModTime      time.Time
	IsCompressed bool
}

// ListAssets returns a list of all embedded static assets
func ListAssets() ([]Asset, error) {
	var assets []Asset

	staticFiles, err := GetStaticFS()
	if err != nil {
		return nil, err
	}

	err = fs.WalkDir(staticFiles, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if !d.IsDir() {
			info, err := d.Info()
			if err != nil {
				return err
			}

			assets = append(assets, Asset{
				Path:    path,
				Size:    info.Size(),
				ModTime: info.ModTime(),
			})
		}

		return nil
	})

	return assets, err
}
