package app

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Gabes-Tint/mega-agents/internal/gitops"
)

func runGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	command := exec.Command("git", append([]string{
		"-c", "user.name=Test", "-c", "user.email=test@example.com",
		"-c", "init.defaultBranch=main", "-c", "commit.gpgsign=false",
	}, args...)...)
	command.Dir = dir
	command.Env = gitops.CleanEnvironment(os.Environ())
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v: %s", args, err, output)
	}
	return strings.TrimSpace(string(output))
}

// projectClone returns a clone whose origin is https://github.com/acme/api,
// rewritten locally to a bare repository that already holds a newer commit
// than the clone has seen. It also returns that newer commit.
func projectClone(t *testing.T) (string, string) {
	t.Helper()
	root := t.TempDir()
	bare := filepath.Join(root, "remote.git")
	seed := filepath.Join(root, "seed")
	clone := filepath.Join(root, "clone")
	runGit(t, root, "init", "--bare", "--quiet", bare)
	runGit(t, root, "init", "--quiet", seed)
	runGit(t, seed, "commit", "--quiet", "--allow-empty", "-m", "first")
	runGit(t, seed, "push", "--quiet", bare, "HEAD:refs/heads/main")
	runGit(t, root, "clone", "--quiet", bare, clone)
	runGit(t, clone, "remote", "set-url", "origin", "https://github.com/acme/api")
	runGit(t, clone, "config", "url."+bare+".insteadOf", "https://github.com/acme/api")
	runGit(t, seed, "commit", "--quiet", "--allow-empty", "-m", "second")
	runGit(t, seed, "push", "--quiet", bare, "HEAD:refs/heads/main")
	return clone, runGit(t, seed, "rev-parse", "HEAD")
}

func runBody(projectPath string, github string) string {
	return fmt.Sprintf(`{
		"nodes": [
			{"id": "p1", "type": "project", "name": "api", "x": 0, "y": 0, "w": 400, "h": 300, "path": %q},
			%s,
			{"id": "a1", "type": "agent", "name": "Agent 1", "x": 200, "y": 10, "w": 160, "h": 64, "parentId": "p1"}
		],
		"edges": [{"id": "e1", "from": "g1", "to": "a1"}]
	}`, projectPath, github)
}

const startedGitHub = `{"id": "g1", "type": "github", "name": "GitHub 1", "x": 10, "y": 10, "w": 160, "h": 64, "parentId": "p1", "start": true, "authenticated": true, "repository": "acme/api"}`

func postRun(t *testing.T, body string) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest(http.MethodPost, "/api/runs", strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	NewHandler(testAssets()).ServeHTTP(response, request)
	return response
}

func decodeRun(t *testing.T, response *httptest.ResponseRecorder) RunResponse {
	t.Helper()
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", response.Code, response.Body.String())
	}
	var run RunResponse
	if err := json.NewDecoder(response.Body).Decode(&run); err != nil {
		t.Fatalf("decode run: %v", err)
	}
	return run
}

func TestRunFetchesTheStartingGitHubBlockInItsProjectPath(t *testing.T) {
	clone, pushed := projectClone(t)

	run := decodeRun(t, postRun(t, runBody(clone, startedGitHub)))

	if run.Status != "succeeded" || len(run.Steps) != 1 {
		t.Fatalf("run = %+v, want one succeeded step", run)
	}
	step := run.Steps[0]
	if step.NodeID != "g1" || step.Name != "GitHub 1" || step.Action != "fetch" ||
		step.Status != "succeeded" || step.Remote != "origin" || step.Error != "" {
		t.Fatalf("step = %+v", step)
	}
	if got := runGit(t, clone, "rev-parse", "refs/remotes/origin/main"); got != pushed {
		t.Fatalf("origin/main = %s, want %s", got, pushed)
	}
}

func TestRunReportsAFailedStepWithTheReason(t *testing.T) {
	notAClone := t.TempDir()
	cases := map[string]struct {
		path   string
		github string
		want   string
	}{
		"not authenticated": {
			path:   notAClone,
			github: strings.Replace(startedGitHub, `"authenticated": true`, `"authenticated": false`, 1),
			want:   `Check "Already authenticated"`,
		},
		"missing repository": {
			path:   notAClone,
			github: strings.Replace(startedGitHub, `"repository": "acme/api"`, `"repository": ""`, 1),
			want:   "Set the repository",
		},
		"missing project path": {
			path:   "",
			github: startedGitHub,
			want:   "Set the path on the project",
		},
		"relative project path": {
			path:   "api",
			github: startedGitHub,
			want:   "must be absolute",
		},
		"path is not a clone": {
			path:   notAClone,
			github: startedGitHub,
			want:   "is not a Git repository",
		},
	}
	for name, testCase := range cases {
		t.Run(name, func(t *testing.T) {
			run := decodeRun(t, postRun(t, runBody(testCase.path, testCase.github)))

			if run.Status != "failed" || len(run.Steps) != 1 || run.Steps[0].Status != "failed" {
				t.Fatalf("run = %+v, want one failed step", run)
			}
			if !strings.Contains(run.Steps[0].Error, testCase.want) {
				t.Fatalf("error = %q, want it to contain %q", run.Steps[0].Error, testCase.want)
			}
		})
	}
}

func TestRunRequiresAStartingGitHubBlock(t *testing.T) {
	github := strings.Replace(startedGitHub, `"start": true`, `"start": false`, 1)

	response := postRun(t, runBody(t.TempDir(), github))

	if response.Code != http.StatusBadRequest ||
		!strings.Contains(response.Body.String(), "flag a GitHub block as the starting point") {
		t.Fatalf("response = %d %q", response.Code, response.Body.String())
	}
}

func TestRunRejectsGraphsTheEditorWouldNotAllow(t *testing.T) {
	body := `{"nodes": [{"id": "g1", "type": "github", "name": "GitHub 1", "start": true, "authenticated": true, "repository": "acme/api"}]}`

	response := postRun(t, body)

	if response.Code != http.StatusBadRequest ||
		!strings.Contains(response.Body.String(), "github cannot be placed inside root") {
		t.Fatalf("response = %d %q", response.Code, response.Body.String())
	}
}

func TestRunRejectsMalformedJSON(t *testing.T) {
	response := postRun(t, "{")

	if response.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", response.Code)
	}
}

// A cross-site form can post text/plain without a CORS preflight; requiring
// JSON makes browsers preflight, which this server never approves.
func TestRunRequiresAJSONContentType(t *testing.T) {
	clone, pushed := projectClone(t)
	before := runGit(t, clone, "rev-parse", "refs/remotes/origin/main")
	request := httptest.NewRequest(http.MethodPost, "/api/runs", strings.NewReader(runBody(clone, startedGitHub)))
	request.Header.Set("Content-Type", "text/plain")
	response := httptest.NewRecorder()

	NewHandler(testAssets()).ServeHTTP(response, request)

	if response.Code != http.StatusUnsupportedMediaType {
		t.Fatalf("status = %d, want 415", response.Code)
	}
	if got := runGit(t, clone, "rev-parse", "refs/remotes/origin/main"); got != before || got == pushed {
		t.Fatalf("origin/main moved to %s; a rejected run must not fetch", got)
	}
}
