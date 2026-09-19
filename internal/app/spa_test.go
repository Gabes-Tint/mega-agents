package app

import (
	"io/fs"
	"net/http"
	"strings"
	"testing"
	"testing/fstest"
)

const indexDocument = "<!doctype html><title>Mega Agents</title>"

// spaAssets is a built frontend: the index document and one hashed asset.
func spaAssets() fs.FS {
	return fstest.MapFS{
		"index.html":             {Data: []byte(indexDocument)},
		"assets/index-abc123.js": {Data: []byte("console.log('editor')\n")},
	}
}

func spaHandler(t *testing.T) http.Handler {
	t.Helper()
	t.Setenv("MEGA_AGENTS_HOME", t.TempDir())
	return NewHandler(spaAssets())
}

func TestDeepLinkToSavedWorkflowServesTheEditor(t *testing.T) {
	handler := spaHandler(t)
	saved := workflowRequest(t, handler, http.MethodPut, "/api/workflows/issue-to-pr", savedGraph, "application/json")
	if saved.Code != http.StatusOK {
		t.Fatalf("save = %d %s", saved.Code, saved.Body.String())
	}

	response := get(t, handler, "/workflows/issue-to-pr")

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusOK)
	}
	if body := response.Body.String(); body != indexDocument {
		t.Fatalf("body = %q, want the index document", body)
	}
	if got := response.Header().Get("Content-Type"); !strings.HasPrefix(got, "text/html") {
		t.Fatalf("content type = %q, want text/html", got)
	}
}

func TestDeepLinkToUnknownWorkflowAnswersNotFoundWithTheEditor(t *testing.T) {
	handler := spaHandler(t)

	response := get(t, handler, "/workflows/missing")

	// The editor names the workflow that does not exist, so the document is
	// served; the status stays honest for anything that is not a browser.
	if response.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusNotFound)
	}
	if body := response.Body.String(); body != indexDocument {
		t.Fatalf("body = %q, want the index document", body)
	}
}

func TestUnknownPageAnswersNotFoundWithTheEditor(t *testing.T) {
	handler := spaHandler(t)

	response := get(t, handler, "/runs/42")

	if response.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusNotFound)
	}
	if body := response.Body.String(); body != indexDocument {
		t.Fatalf("body = %q, want the index document", body)
	}
}

func TestMissingAssetStaysNotFoundAndIsNotTheEditor(t *testing.T) {
	handler := spaHandler(t)

	response := get(t, handler, "/assets/index-deleted.js")

	if response.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusNotFound)
	}
	// A broken build must fail loudly instead of answering HTML.
	if body := response.Body.String(); strings.Contains(body, "<!doctype html>") {
		t.Fatalf("body = %q, want a plain not found", body)
	}
	if got := response.Header().Get("Content-Type"); strings.HasPrefix(got, "text/html") && response.Body.Len() > 64 {
		t.Fatalf("content type = %q, want no editor document", got)
	}
}

func TestBuiltAssetIsStillServed(t *testing.T) {
	handler := spaHandler(t)

	response := get(t, handler, "/assets/index-abc123.js")

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusOK)
	}
	if body := response.Body.String(); body != "console.log('editor')\n" {
		t.Fatalf("body = %q, want the built asset", body)
	}
	if got := response.Header().Get("Content-Type"); !strings.Contains(got, "javascript") {
		t.Fatalf("content type = %q, want JavaScript", got)
	}
}

func TestUnknownAPIPathStaysNotFoundAndIsNotTheEditor(t *testing.T) {
	handler := spaHandler(t)

	response := get(t, handler, "/api/nothing-here")

	if response.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusNotFound)
	}
	if body := response.Body.String(); strings.Contains(body, "<!doctype html>") {
		t.Fatalf("body = %q, want a plain not found", body)
	}
}

// The list of workflows and their schedules is a page of the editor, so a
// link straight to it is served like any other.
func TestLinkToTheWorkflowListServesTheEditor(t *testing.T) {
	handler := spaHandler(t)

	response := get(t, handler, "/workflows")

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusOK)
	}
	if body := response.Body.String(); body != indexDocument {
		t.Fatalf("body = %q, want the index document", body)
	}
}
