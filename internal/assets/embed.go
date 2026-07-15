// Package assets embeds and serves RunRun's static web assets (CSS, JS,
// icons) directly from the compiled binary, with no external files or
// Node toolchain required at runtime.
package assets

import (
	"embed"
	"fmt"
	"io/fs"
	"net/http"
	"strings"
	"time"
)

// Cache lifetimes applied to embedded static assets, in seconds.
const (
	// cacheMaxAgeAssets is applied to CSS/JS, which may change frequently
	// during development. In production, longer cache times with
	// versioned filenames would be preferable.
	cacheMaxAgeAssets = 3600 // 1 hour

	// cacheMaxAgeMedia is applied to images and fonts, which change rarely.
	cacheMaxAgeMedia = 604800 // 1 week
)

// Static assets embedded into the binary
//
//go:embed all:static
var staticFS embed.FS

// GetStaticFS returns the embedded filesystem for static assets.
func GetStaticFS() (fs.FS, error) {
	// Return a sub-filesystem starting at "static" directory
	sub, err := fs.Sub(staticFS, "static")
	if err != nil {
		return nil, fmt.Errorf("get static sub-filesystem: %w", err)
	}
	return sub, nil
}

// NewStaticFileServer creates an HTTP handler that serves static files
// from the embedded filesystem with proper cache headers.
func NewStaticFileServer() http.Handler {
	staticFiles, err := GetStaticFS()
	if err != nil {
		panic(err)
	}

	fileServer := http.FileServer(http.FS(staticFiles))

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		setCacheHeaders(w, path)
		setContentTypeHeaders(w, path)

		// Serve the file
		fileServer.ServeHTTP(w, r)
	})
}

// setCacheHeaders sets a Cache-Control header appropriate to the asset
// type identified by path's extension.
func setCacheHeaders(w http.ResponseWriter, path string) {
	switch {
	case strings.HasSuffix(path, ".css"), strings.HasSuffix(path, ".js"):
		w.Header().Set("Cache-Control", fmt.Sprintf("public, max-age=%d", cacheMaxAgeAssets))
	case isMediaAsset(path):
		w.Header().Set("Cache-Control", fmt.Sprintf("public, max-age=%d", cacheMaxAgeMedia))
	}
}

// isMediaAsset reports whether path names an image or font asset.
func isMediaAsset(path string) bool {
	mediaSuffixes := []string{
		".png", ".jpg", ".jpeg", ".gif", ".svg", ".ico",
		".woff", ".woff2", ".ttf", ".eot",
	}
	for _, suffix := range mediaSuffixes {
		if strings.HasSuffix(path, suffix) {
			return true
		}
	}
	return false
}

// setContentTypeHeaders sets an explicit Content-Type header for asset
// types where http.FileServer's built-in sniffing is not desired.
func setContentTypeHeaders(w http.ResponseWriter, path string) {
	switch {
	case strings.HasSuffix(path, ".css"):
		w.Header().Set("Content-Type", "text/css; charset=utf-8")
	case strings.HasSuffix(path, ".js"):
		w.Header().Set("Content-Type", "application/javascript; charset=utf-8")
	}
}

// Asset represents metadata about an embedded asset.
type Asset struct {
	Path         string
	Size         int64
	ModTime      time.Time
	IsCompressed bool
}

// ListAssets returns a list of all embedded static assets.
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
				return fmt.Errorf("get file info for %s: %w", path, err)
			}

			assets = append(assets, Asset{
				Path:    path,
				Size:    info.Size(),
				ModTime: info.ModTime(),
			})
		}

		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("walk static assets: %w", err)
	}

	return assets, nil
}
