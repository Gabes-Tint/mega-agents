package app

import (
	"io/fs"
	"net/http"
	"path"
	"strconv"
	"strings"
)

// spaFileServer serves the built editor. Files that exist are served as they
// are; a deep link such as /workflows/issue-to-pr has no file of its own, so
// the index document is served and the editor opens what the address names.
//
// A path that looks like a file, and anything under /api/ no endpoint claimed,
// stays a plain 404: answering HTML with a 200 there would hide a broken build
// behind a page that silently loads nothing.
func spaFileServer(assets fs.FS, workflows WorkflowStore) http.Handler {
	files := http.FileServer(http.FS(assets))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if hasAsset(assets, r.URL.Path) {
			files.ServeHTTP(w, r)
			return
		}
		requested := path.Clean("/" + strings.TrimPrefix(r.URL.Path, "/"))
		if r.Method != http.MethodGet && r.Method != http.MethodHead ||
			strings.HasPrefix(requested, "/api/") ||
			strings.Contains(path.Base(requested), ".") {
			http.NotFound(w, r)
			return
		}
		// The editor still draws the page, so it can name the workflow that
		// does not exist, while the status code stays honest for everything
		// that is not a browser.
		status := http.StatusNotFound
		if editorPageExists(requested, workflows) {
			status = http.StatusOK
		}
		serveIndex(w, assets, status)
	})
}

// hasAsset reports whether the request names a file of the built frontend.
func hasAsset(assets fs.FS, requested string) bool {
	name := strings.TrimPrefix(path.Clean("/"+strings.TrimPrefix(requested, "/")), "/")
	if name == "" {
		name = "index.html"
	}
	info, err := fs.Stat(assets, name)
	return err == nil && !info.IsDir()
}

// editorPageExists reports whether the editor has something to show at the
// path: the scratch graph at the root, the list of saved workflows and their
// schedules, or a saved workflow.
func editorPageExists(requested string, workflows WorkflowStore) bool {
	if requested == "/" || requested == "/workflows" {
		return true
	}
	name, isWorkflow := strings.CutPrefix(requested, "/workflows/")
	return isWorkflow && !strings.Contains(name, "/") && workflows.Exists(name)
}

func serveIndex(w http.ResponseWriter, assets fs.FS, status int) {
	document, err := fs.ReadFile(assets, "index.html")
	if err != nil {
		http.Error(w, "the editor is not built", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Content-Length", strconv.Itoa(len(document)))
	w.WriteHeader(status)
	_, _ = w.Write(document)
}
