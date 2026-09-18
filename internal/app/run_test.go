package app

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Gabes-Tint/mega-agents/internal/engine"
	"github.com/Gabes-Tint/mega-agents/internal/gitops"
	"github.com/Gabes-Tint/mega-agents/internal/runs"
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

// postRun starts a run on a handler recording under a fresh Mega Agents
// home, which the returned handler keeps using.
func postRun(t *testing.T, body string) (*httptest.ResponseRecorder, http.Handler) {
	t.Helper()
	t.Setenv("MEGA_AGENTS_HOME", t.TempDir())
	handler := NewHandler(testAssets())
	request := httptest.NewRequest(http.MethodPost, "/api/runs", strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	return response, handler
}

func finishedRunOnly(t *testing.T, body string) runs.Record {
	t.Helper()
	record, _ := finishedRun(t, body)
	return record
}

func get(t *testing.T, handler http.Handler, path string) *httptest.ResponseRecorder {
	t.Helper()
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, path, nil))
	return response
}

func decodeRecord(t *testing.T, response *httptest.ResponseRecorder) runs.Record {
	t.Helper()
	var record runs.Record
	if err := json.NewDecoder(response.Body).Decode(&record); err != nil {
		t.Fatalf("decode run: %v: %s", err, response.Body.String())
	}
	return record
}

// finishedRun starts a run and follows it until it is no longer running.
func finishedRun(t *testing.T, body string) (runs.Record, http.Handler) {
	t.Helper()
	response, handler := postRun(t, body)
	if response.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want 202: %s", response.Code, response.Body.String())
	}
	return awaitRun(t, handler, decodeRecord(t, response).ID), handler
}

// awaitRun follows a run until it is no longer running, so its background
// execution never outlives the test's temporary folders.
func awaitRun(t *testing.T, handler http.Handler, id string) runs.Record {
	t.Helper()
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		polled := get(t, handler, "/api/runs/"+id)
		if polled.Code != http.StatusOK {
			t.Fatalf("poll = %d: %s", polled.Code, polled.Body.String())
		}
		// A run paused at a breakpoint is still in progress: it ends only
		// once someone takes it on.
		if record := decodeRecord(t, polled); record.Status != engine.Running && record.Status != Paused {
			return record
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("run %s did not finish", id)
	return runs.Record{}
}

func TestRunFetchesTheStartingGitHubBlockInItsProjectPath(t *testing.T) {
	clone, pushed := projectClone(t)

	run := finishedRunOnly(t, runBody(clone, startedGitHub))

	if run.Status != "succeeded" || len(run.Steps) != 1 {
		t.Fatalf("run = %+v, want one succeeded step", run)
	}
	step := run.Steps[0]
	if step.TaskID != "g1" || step.Name != "GitHub 1" || step.Kind != "fetch" ||
		step.Status != "succeeded" || step.Details["remote"] != "origin" || step.Error != "" {
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
			want:   `check "Already authenticated"`,
		},
		"missing project path": {
			path:   "",
			github: startedGitHub,
			want:   "set the path on the project",
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
			run := finishedRunOnly(t, runBody(testCase.path, testCase.github))

			if run.Status != "failed" || len(run.Steps) != 1 || run.Steps[0].Status != "failed" {
				t.Fatalf("run = %+v, want one failed step", run)
			}
			if !strings.Contains(run.Steps[0].Error, testCase.want) {
				t.Fatalf("error = %q, want it to contain %q", run.Steps[0].Error, testCase.want)
			}
		})
	}
}

func TestRunLoadsAnEmptyRepositoryFromTheProject(t *testing.T) {
	clone, pushed := projectClone(t)
	github := strings.Replace(startedGitHub, `"repository": "acme/api"`, `"repository": ""`, 1)

	run := finishedRunOnly(t, runBody(clone, github))

	if run.Status != "succeeded" || len(run.Steps) != 1 || run.Steps[0].Details["repository"] != "acme/api" {
		t.Fatalf("run = %+v, want one succeeded step for acme/api", run)
	}
	if got := runGit(t, clone, "rev-parse", "refs/remotes/origin/main"); got != pushed {
		t.Fatalf("origin/main = %s, want %s", got, pushed)
	}
}

func TestRunFailsAnEmptyRepositoryWhenTheProjectHasNoGitHubRemote(t *testing.T) {
	clone, _ := projectClone(t)
	runGit(t, clone, "remote", "set-url", "origin", "https://gitlab.com/acme/api.git")
	github := strings.Replace(startedGitHub, `"repository": "acme/api"`, `"repository": ""`, 1)

	run := finishedRunOnly(t, runBody(clone, github))

	if run.Status != "failed" || len(run.Steps) != 1 ||
		!strings.Contains(run.Steps[0].Error, "no GitHub remote") ||
		!strings.Contains(run.Steps[0].Error, "set the repository on the GitHub block") {
		t.Fatalf("run = %+v, want a failed step asking for the repository", run)
	}
}

func getRepository(t *testing.T, path string) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest(http.MethodGet, "/api/git/repository?path="+url.QueryEscape(path), nil)
	response := httptest.NewRecorder()
	NewHandler(testAssets()).ServeHTTP(response, request)
	return response
}

func TestRepositoryEndpointReportsTheProjectsGitHubRepository(t *testing.T) {
	clone, _ := projectClone(t)

	response := getRepository(t, clone)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", response.Code, response.Body.String())
	}
	var body gitops.DetectedRepository
	if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Repository != "acme/api" || body.Remote != "origin" {
		t.Fatalf("body = %+v, want acme/api from origin", body)
	}
}

func TestRepositoryEndpointRejectsPathsWithoutAGitHubRepository(t *testing.T) {
	cases := map[string]string{
		"not a clone":   t.TempDir(),
		"empty path":    "",
		"relative path": "api",
	}
	for name, path := range cases {
		t.Run(name, func(t *testing.T) {
			response := getRepository(t, path)

			if response.Code != http.StatusNotFound || response.Body.Len() == 0 {
				t.Fatalf("response = %d %q, want 404 with a reason", response.Code, response.Body.String())
			}
		})
	}
}

func TestRunRequiresAStartingGitHubBlock(t *testing.T) {
	github := strings.Replace(startedGitHub, `"start": true`, `"start": false`, 1)

	response, _ := postRun(t, runBody(t.TempDir(), github))

	if response.Code != http.StatusBadRequest ||
		!strings.Contains(response.Body.String(), "flag a GitHub block or an agent as the starting point") {
		t.Fatalf("response = %d %q", response.Code, response.Body.String())
	}
}

func TestRunRejectsGraphsTheEditorWouldNotAllow(t *testing.T) {
	body := `{"nodes": [{"id": "g1", "type": "github", "name": "GitHub 1", "start": true, "authenticated": true, "repository": "acme/api"}]}`

	response, _ := postRun(t, body)

	if response.Code != http.StatusBadRequest ||
		!strings.Contains(response.Body.String(), "github cannot be placed inside root") {
		t.Fatalf("response = %d %q", response.Code, response.Body.String())
	}
}

func TestRunRejectsMalformedJSON(t *testing.T) {
	response, _ := postRun(t, "{")

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

// actionFlow is a project holding a started GitHub block whose own actions
// are the given nodes and sequence edges.
func actionFlow(projectPath string, actions string, edges string) string {
	return fmt.Sprintf(`{
		"nodes": [
			{"id": "p1", "type": "project", "name": "api", "path": %q},
			{"id": "g1", "type": "github", "name": "GitHub 1", "parentId": "p1", "start": true, "authenticated": true},
			%s
		],
		"edges": [%s]
	}`, projectPath, actions, edges)
}

func TestRunExecutesAGitHubBlocksActionsInSequence(t *testing.T) {
	clone, pushed := projectClone(t)
	worktree := filepath.Join(t.TempDir(), "login")
	body := actionFlow(clone, fmt.Sprintf(`
		{"id": "a3", "type": "action", "action": "rebase", "name": "Rebase", "parentId": "g1"},
		{"id": "a1", "type": "action", "action": "fetch", "name": "Fetch", "parentId": "g1", "start": true},
		{"id": "a2", "type": "action", "action": "worktree", "name": "Create worktree", "parentId": "g1", "branch": "feature/login", "worktreePath": %q}`,
		worktree), `{"id": "e1", "from": "a1", "to": "a2"}, {"id": "e2", "from": "a2", "to": "a3"}`)

	run := finishedRunOnly(t, body)

	if run.Status != engine.Succeeded {
		t.Fatalf("run = %+v", run)
	}
	var kinds []string
	for _, step := range run.Steps {
		if step.Status == engine.Succeeded {
			kinds = append(kinds, step.Kind)
		}
	}
	if strings.Join(kinds, ",") != "rebase,fetch,worktree" || len(run.Steps) != 3 {
		t.Fatalf("steps = %+v, want all three actions to succeed", run.Steps)
	}
	created := run.Steps[2].Details
	if created["path"] != worktree || created["branch"] != "feature/login" || created["baseRevision"] != pushed {
		t.Fatalf("worktree details = %+v, want the branch cut from the fetched %s", created, pushed)
	}
	// The worktree starts from what fetch brought in, which proves the fetch
	// ran before it even though the rebase was declared first.
	if got := runGit(t, worktree, "rev-parse", "HEAD"); got != pushed {
		t.Fatalf("worktree HEAD = %s, want %s", got, pushed)
	}
	if run.Steps[0].Details["path"] != worktree {
		t.Fatalf("rebase details = %+v, want the workspace it rebased", run.Steps[0].Details)
	}
}

func TestRunSkipsTheActionsAfterAFailedOne(t *testing.T) {
	clone, _ := projectClone(t)
	body := actionFlow(clone, `
		{"id": "a1", "type": "action", "action": "worktree", "name": "Create worktree", "parentId": "g1", "start": true},
		{"id": "a2", "type": "action", "action": "rebase", "name": "Rebase", "parentId": "g1"}`,
		`{"id": "e1", "from": "a1", "to": "a2"}`)

	run := finishedRunOnly(t, body)

	if run.Status != engine.Failed || run.Steps[0].Status != engine.Failed || run.Steps[1].Status != engine.Skipped {
		t.Fatalf("run = %+v, want the worktree to fail and the rebase to be skipped", run)
	}
	if !strings.Contains(run.Steps[0].Error, "set the branch") {
		t.Fatalf("error = %q", run.Steps[0].Error)
	}
}

func TestRunOnlyExecutesActionsReachableFromTheStartingAction(t *testing.T) {
	clone, _ := projectClone(t)
	t.Setenv("MEGA_AGENTS_HOME", t.TempDir())
	body := actionFlow(clone, `
		{"id": "a1", "type": "action", "action": "fetch", "name": "Fetch", "parentId": "g1", "start": true},
		{"id": "a2", "type": "action", "action": "worktree", "name": "Loose worktree", "parentId": "g1", "branch": "loose"}`, ``)

	run := finishedRunOnly(t, body)

	if len(run.Steps) != 1 || run.Steps[0].Kind != "fetch" {
		t.Fatalf("steps = %+v, want only the connected fetch", run.Steps)
	}
}

func TestRunRejectsActionFlowsThatCannotRun(t *testing.T) {
	cases := map[string]struct {
		actions string
		edges   string
		want    string
	}{
		"no starting action": {
			actions: `{"id": "a1", "type": "action", "action": "fetch", "name": "Fetch", "parentId": "g1"}`,
			want:    "flag the first action of GitHub 1 as its starting point",
		},
		"rebase without a workspace": {
			actions: `{"id": "a1", "type": "action", "action": "fetch", "name": "Fetch", "parentId": "g1", "start": true},
				{"id": "a2", "type": "action", "action": "rebase", "name": "Rebase", "parentId": "g1"}`,
			edges: `{"id": "e1", "from": "a1", "to": "a2"}`,
			want:  "Rebase needs a workspace; connect a Create worktree action before it",
		},
		"unknown action": {
			actions: `{"id": "a1", "type": "action", "action": "deploy", "name": "Deploy", "parentId": "g1", "start": true}`,
			want:    `unknown Git action "deploy"`,
		},
		"cycle": {
			actions: `{"id": "a1", "type": "action", "action": "fetch", "name": "Fetch", "parentId": "g1", "start": true},
				{"id": "a2", "type": "action", "action": "fetch", "name": "Again", "parentId": "g1"}`,
			edges: `{"id": "e1", "from": "a1", "to": "a2"}, {"id": "e2", "from": "a2", "to": "a1"}`,
			want:  "cycle",
		},
	}
	for name, testCase := range cases {
		t.Run(name, func(t *testing.T) {
			response, _ := postRun(t, actionFlow(t.TempDir(), testCase.actions, testCase.edges))

			if response.Code != http.StatusBadRequest || !strings.Contains(response.Body.String(), testCase.want) {
				t.Fatalf("response = %d %q, want 400 containing %q", response.Code, response.Body.String(), testCase.want)
			}
		})
	}
}

func TestActionsOnlyLiveInsideAGitHubBlock(t *testing.T) {
	body := `{"nodes": [
		{"id": "p1", "type": "project", "name": "api"},
		{"id": "a1", "type": "action", "action": "fetch", "name": "Fetch", "parentId": "p1", "start": true}
	]}`

	response, _ := postRun(t, body)

	if response.Code != http.StatusBadRequest || !strings.Contains(response.Body.String(), "action cannot be placed inside project") {
		t.Fatalf("response = %d %q", response.Code, response.Body.String())
	}
}

func TestRunKeepsTheLogOfEveryStep(t *testing.T) {
	clone, _ := projectClone(t)

	record, handler := finishedRun(t, runBody(clone, startedGitHub))

	response := get(t, handler, "/api/runs/"+record.ID+"/logs/g1")
	if response.Code != http.StatusOK || !strings.HasPrefix(response.Header().Get("Content-Type"), "text/plain") {
		t.Fatalf("log response = %d %q", response.Code, response.Header().Get("Content-Type"))
	}
	log := response.Body.String()
	for _, want := range []string{"▶️ GitHub 1 started at ", "$ git fetch origin\n", "main       -> origin/main", "✅ GitHub 1 succeeded in "} {
		if !strings.Contains(log, want) {
			t.Fatalf("log = %q, want it to contain %q", log, want)
		}
	}
}

func TestRunAnswersAtOnceWithThePlannedSteps(t *testing.T) {
	clone, _ := projectClone(t)

	response, handler := postRun(t, runBody(clone, startedGitHub))

	if response.Code != http.StatusAccepted {
		t.Fatalf("status = %d: %s", response.Code, response.Body.String())
	}
	record := decodeRecord(t, response)
	if record.ID == "" || record.Status != engine.Running || len(record.Steps) != 1 ||
		record.Steps[0].TaskID != "g1" || record.Steps[0].Status != engine.Pending {
		t.Fatalf("record = %+v", record)
	}
	awaitRun(t, handler, record.ID)
}

func TestRunsAreListedNewestFirst(t *testing.T) {
	clone, _ := projectClone(t)
	first, handler := finishedRun(t, runBody(clone, startedGitHub))

	response := get(t, handler, "/api/runs")

	var records []runs.Record
	if err := json.NewDecoder(response.Body).Decode(&records); err != nil {
		t.Fatal(err)
	}
	if response.Code != http.StatusOK || len(records) != 1 || records[0].ID != first.ID || records[0].Workflow != "workflow" {
		t.Fatalf("list = %d %+v", response.Code, records)
	}
}

func TestUnknownRunsAndLogsAreNotFound(t *testing.T) {
	clone, _ := projectClone(t)
	record, handler := finishedRun(t, runBody(clone, startedGitHub))

	for _, path := range []string{
		"/api/runs/20000101T000000Z-000000",
		"/api/runs/" + record.ID + "/logs/a1",
		"/api/runs/" + record.ID + "/logs/.hidden",
	} {
		if response := get(t, handler, path); response.Code != http.StatusNotFound {
			t.Errorf("GET %s = %d, want 404", path, response.Code)
		}
	}
}

func jsonUnmarshal(text string, value any) error {
	return json.Unmarshal([]byte(text), value)
}
