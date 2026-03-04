package assets

import (
	"io/fs"
	"testing"
)

func TestGetStaticFS(t *testing.T) {
	staticFS, err := GetStaticFS()
	if err != nil {
		t.Fatalf("GetStaticFS() error: %v", err)
	}
	if staticFS == nil {
		t.Fatal("GetStaticFS() returned nil")
	}
}

func TestListAssets(t *testing.T) {
	assets, err := ListAssets()
	if err != nil {
		t.Fatalf("ListAssets() error: %v", err)
	}
	if len(assets) == 0 {
		t.Fatal("ListAssets() returned no assets")
	}

	// Build a set of embedded paths
	paths := make(map[string]bool)
	for _, a := range assets {
		paths[a.Path] = true
	}

	// Verify all expected assets are embedded
	expected := []string{
		"css/styles.css",
		"css/input.css",
		"js/main.js",
		"js/log-viewer.js",
		"js/vendor/ansi_up.min.js",
		"js/vendor/ansi_up_loader.js",
		"favicon.ico",
	}

	for _, path := range expected {
		if !paths[path] {
			t.Errorf("expected asset %q not found in embedded filesystem", path)
		}
	}
}

func TestStaticFSReadFiles(t *testing.T) {
	staticFS, err := GetStaticFS()
	if err != nil {
		t.Fatalf("GetStaticFS() error: %v", err)
	}

	files := []string{
		"css/styles.css",
		"js/main.js",
		"js/log-viewer.js",
		"favicon.ico",
	}

	for _, file := range files {
		data, err := fs.ReadFile(staticFS, file)
		if err != nil {
			t.Errorf("failed to read embedded file %q: %v", file, err)
			continue
		}
		if len(data) == 0 {
			t.Errorf("embedded file %q is empty", file)
		}
	}
}

func TestNewStaticFileServer(t *testing.T) {
	handler := NewStaticFileServer()
	if handler == nil {
		t.Fatal("NewStaticFileServer() returned nil")
	}
}
