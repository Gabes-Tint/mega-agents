package app

import (
	"encoding/json"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"testing"
	"testing/fstest"
)

func testAssets() fs.FS {
	return fstest.MapFS{
		"index.html": {Data: []byte("<!doctype html><title>Mega Agents</title>")},
	}
}

func TestStatusEndpoint(t *testing.T) {
	t.Parallel()

	request := httptest.NewRequest(http.MethodGet, "/api/status", nil)
	response := httptest.NewRecorder()
	NewHandler(testAssets()).ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusOK)
	}
	if got := response.Header().Get("Content-Type"); got != "application/json" {
		t.Fatalf("content type = %q, want application/json", got)
	}

	var body StatusResponse
	if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body.Message != "Mega Agents backend is running" {
		t.Fatalf("message = %q", body.Message)
	}
}

func TestFrontendAssets(t *testing.T) {
	t.Parallel()

	request := httptest.NewRequest(http.MethodGet, "/", nil)
	response := httptest.NewRecorder()
	NewHandler(testAssets()).ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusOK)
	}
	if body := response.Body.String(); body != "<!doctype html><title>Mega Agents</title>" {
		t.Fatalf("unexpected frontend response: %q", body)
	}
}

func TestDirectoriesEndpointListsSubdirectories(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "agents"), 0o755); err != nil {
		t.Fatalf("create subdirectory: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "notes.txt"), []byte("x"), 0o644); err != nil {
		t.Fatalf("create file: %v", err)
	}

	request := httptest.NewRequest(http.MethodGet, "/api/directories?path="+url.QueryEscape(root), nil)
	response := httptest.NewRecorder()
	NewHandler(testAssets()).ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d: %s", response.Code, http.StatusOK, response.Body.String())
	}
	var body DirectoriesResponse
	if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body.Path != root {
		t.Fatalf("path = %q, want %q", body.Path, root)
	}
	if len(body.Directories) != 1 || body.Directories[0] != "agents" {
		t.Fatalf("directories = %v, want [agents] (files must be excluded)", body.Directories)
	}
}

func TestDirectoriesEndpointRejectsUnreadablePath(t *testing.T) {
	t.Parallel()

	request := httptest.NewRequest(http.MethodGet, "/api/directories?path=/definitely-missing-dir-xyz", nil)
	response := httptest.NewRecorder()
	NewHandler(testAssets()).ServeHTTP(response, request)

	if response.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusBadRequest)
	}
}

func TestDirectoriesEndpointDefaultsToHome(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	request := httptest.NewRequest(http.MethodGet, "/api/directories", nil)
	response := httptest.NewRecorder()
	NewHandler(testAssets()).ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d: %s", response.Code, http.StatusOK, response.Body.String())
	}
	var body DirectoriesResponse
	if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body.Path != home {
		t.Fatalf("path = %q, want home %q", body.Path, home)
	}
}
