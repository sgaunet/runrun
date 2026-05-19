package assets

import (
	"io/fs"
	"net/http"
	"net/http/httptest"
	"strings"
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

// requiredEmbeddedAssets lists every static asset the application
// expects to find in the embedded filesystem after the Tailwind→Bulma
// migration. See specs/001-replace-tailwind-bulma/contracts/static-asset-urls.md.
func requiredEmbeddedAssets() []struct {
	Path        string
	ContentType string
} {
	return []struct {
		Path        string
		ContentType string
	}{
		{"css/bulma.min.css", "text/css"},
		{"css/app.css", "text/css"},
		{"js/vendor/alpine.csp.min.js", "javascript"},
		{"js/main.js", "javascript"},
		{"js/log-viewer.js", "javascript"},
		{"js/vendor/ansi_up.js", "javascript"},
		{"js/vendor/ansi_up.min.js", "javascript"},
		{"js/vendor/ansi_up_loader.js", "javascript"},
		{"icons/sprite.svg", "image/svg+xml"},
		{"favicon.ico", "icon"},
	}
}

// forbiddenEmbeddedAssets lists legacy Tailwind artifacts that MUST NOT
// be present in the embedded filesystem after the migration. The audit
// test asserts that every URL under this list returns 404.
func forbiddenEmbeddedAssets() []string {
	return []string{
		"css/styles.css",
		"css/input.css",
		"css/style.css",
	}
}

func TestListAssetsContainsRequired(t *testing.T) {
	assets, err := ListAssets()
	if err != nil {
		t.Fatalf("ListAssets() error: %v", err)
	}
	if len(assets) == 0 {
		t.Fatal("ListAssets() returned no assets")
	}
	paths := make(map[string]bool, len(assets))
	for _, a := range assets {
		paths[a.Path] = true
	}
	for _, want := range requiredEmbeddedAssets() {
		if !paths[want.Path] {
			t.Errorf("expected asset %q not found in embedded filesystem", want.Path)
		}
	}
}

func TestListAssetsDoesNotContainForbidden(t *testing.T) {
	assets, err := ListAssets()
	if err != nil {
		t.Fatalf("ListAssets() error: %v", err)
	}
	paths := make(map[string]bool, len(assets))
	for _, a := range assets {
		paths[a.Path] = true
	}
	for _, forbidden := range forbiddenEmbeddedAssets() {
		if paths[forbidden] {
			t.Errorf("forbidden legacy asset %q still present in embedded filesystem", forbidden)
		}
	}
}

func TestStaticFSReadRequiredFiles(t *testing.T) {
	staticFS, err := GetStaticFS()
	if err != nil {
		t.Fatalf("GetStaticFS() error: %v", err)
	}
	for _, want := range requiredEmbeddedAssets() {
		data, err := fs.ReadFile(staticFS, want.Path)
		if err != nil {
			t.Errorf("failed to read embedded file %q: %v", want.Path, err)
			continue
		}
		if len(data) == 0 {
			t.Errorf("embedded file %q is empty", want.Path)
		}
	}
}

func TestEmbeddedAssetURLsServe(t *testing.T) {
	handler := NewStaticFileServer()
	srv := httptest.NewServer(http.StripPrefix("/static", handler))
	t.Cleanup(srv.Close)

	for _, want := range requiredEmbeddedAssets() {
		url := srv.URL + "/static/" + want.Path
		resp, err := http.Get(url) //nolint:gosec // test server URL
		if err != nil {
			t.Errorf("GET %s: %v", url, err)
			continue
		}
		if resp.StatusCode != http.StatusOK {
			t.Errorf("GET %s: status = %d, want 200", url, resp.StatusCode)
			_ = resp.Body.Close()
			continue
		}
		ct := resp.Header.Get("Content-Type")
		if !strings.Contains(strings.ToLower(ct), want.ContentType) {
			t.Errorf("GET %s: Content-Type = %q, want substring %q", url, ct, want.ContentType)
		}
		_ = resp.Body.Close()
	}
}

func TestForbiddenLegacyURLsReturn404(t *testing.T) {
	handler := NewStaticFileServer()
	srv := httptest.NewServer(http.StripPrefix("/static", handler))
	t.Cleanup(srv.Close)

	for _, forbidden := range forbiddenEmbeddedAssets() {
		url := srv.URL + "/static/" + forbidden
		resp, err := http.Get(url) //nolint:gosec // test server URL
		if err != nil {
			t.Errorf("GET %s: %v", url, err)
			continue
		}
		if resp.StatusCode != http.StatusNotFound {
			t.Errorf("GET %s: status = %d, want 404 (forbidden legacy asset)",
				url, resp.StatusCode)
		}
		_ = resp.Body.Close()
	}
}

func TestNewStaticFileServer(t *testing.T) {
	handler := NewStaticFileServer()
	if handler == nil {
		t.Fatal("NewStaticFileServer() returned nil")
	}
}
