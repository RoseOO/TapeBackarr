package api

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
)

func TestStaticFileServing(t *testing.T) {
	// Create a temp directory with static files
	staticDir := t.TempDir()

	// Create an index.html
	indexContent := []byte("<html><body>TapeBackarr</body></html>")
	if err := os.WriteFile(filepath.Join(staticDir, "index.html"), indexContent, 0644); err != nil {
		t.Fatalf("failed to create index.html: %v", err)
	}

	// Create a CSS file in a subdirectory
	cssDir := filepath.Join(staticDir, "_app", "css")
	if err := os.MkdirAll(cssDir, 0755); err != nil {
		t.Fatalf("failed to create css dir: %v", err)
	}
	cssContent := []byte("body { margin: 0; }")
	if err := os.WriteFile(filepath.Join(cssDir, "style.css"), cssContent, 0644); err != nil {
		t.Fatalf("failed to create style.css: %v", err)
	}

	// Create a minimal server with just the static file serving
	s := &Server{
		router:    chi.NewRouter(),
		staticDir: staticDir,
	}
	s.setupRoutes()

	tests := []struct {
		name       string
		path       string
		wantStatus int
		wantBody   string
	}{
		{
			name:       "root serves index.html",
			path:       "/",
			wantStatus: http.StatusOK,
			wantBody:   "TapeBackarr",
		},
		{
			name:       "static CSS file",
			path:       "/_app/css/style.css",
			wantStatus: http.StatusOK,
			wantBody:   "body { margin: 0; }",
		},
		{
			name:       "SPA fallback for unknown path",
			path:       "/dashboard",
			wantStatus: http.StatusOK,
			wantBody:   "TapeBackarr",
		},
		{
			name:       "API route still returns 404",
			path:       "/api/v1/nonexistent",
			wantStatus: http.StatusNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", tt.path, nil)
			rr := httptest.NewRecorder()
			s.router.ServeHTTP(rr, req)

			if rr.Code != tt.wantStatus {
				t.Errorf("expected status %d, got %d", tt.wantStatus, rr.Code)
			}

			if tt.wantBody != "" {
				body := rr.Body.String()
				if !strings.Contains(body, tt.wantBody) {
					t.Errorf("expected body to contain %q, got %q", tt.wantBody, body)
				}
			}
		})
	}
}

func TestNoStaticDir(t *testing.T) {
	// Server with empty staticDir should return 404 for root
	s := &Server{
		router:    chi.NewRouter(),
		staticDir: "",
	}
	s.setupRoutes()

	req := httptest.NewRequest("GET", "/", nil)
	rr := httptest.NewRecorder()
	s.router.ServeHTTP(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Errorf("expected 404 for root with no static dir, got %d", rr.Code)
	}
}
