package gitops

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func runGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	command := exec.Command("git", append([]string{
		"-c", "user.name=Test", "-c", "user.email=test@example.com",
		"-c", "init.defaultBranch=main", "-c", "commit.gpgsign=false",
	}, args...)...)
	command.Dir = dir
	command.Env = CleanEnvironment(os.Environ())
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v: %s", args, err, output)
	}
	return strings.TrimSpace(string(output))
}

// githubClone creates a clone whose origin is spelled as a GitHub URL but is
// rewritten through insteadOf to a local bare repository, so fetch runs
// against real Git without the network. It returns the clone and a seeding
// work tree that pushes to the same bare repository.
func githubClone(t *testing.T, remoteURL string) (string, string) {
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
	runGit(t, clone, "remote", "set-url", "origin", remoteURL)
	runGit(t, clone, "config", "url."+bare+".insteadOf", remoteURL)
	return clone, seed
}

func TestFetchUpdatesTheMatchingRemote(t *testing.T) {
	clone, seed := githubClone(t, "git@github.com:Acme/API.git")
	runGit(t, seed, "commit", "--quiet", "--allow-empty", "-m", "second")
	pushed := runGit(t, seed, "rev-parse", "HEAD")
	runGit(t, seed, "push", "--quiet", filepath.Join(filepath.Dir(seed), "remote.git"), "HEAD:refs/heads/main")

	result, err := Fetch(context.Background(), clone, "acme/api")
	if err != nil {
		t.Fatalf("fetch: %v", err)
	}
	if result.Remote != "origin" {
		t.Fatalf("remote = %q, want origin", result.Remote)
	}
	if got := runGit(t, clone, "rev-parse", "refs/remotes/origin/main"); got != pushed {
		t.Fatalf("origin/main = %s, want the pushed commit %s", got, pushed)
	}
}

func TestFetchLeavesTheCheckoutUntouched(t *testing.T) {
	clone, seed := githubClone(t, "https://github.com/acme/api")
	before := runGit(t, clone, "rev-parse", "HEAD")
	runGit(t, seed, "commit", "--quiet", "--allow-empty", "-m", "second")
	runGit(t, seed, "push", "--quiet", filepath.Join(filepath.Dir(seed), "remote.git"), "HEAD:refs/heads/main")

	if _, err := Fetch(context.Background(), clone, "https://github.com/acme/api.git"); err != nil {
		t.Fatalf("fetch: %v", err)
	}
	if after := runGit(t, clone, "rev-parse", "HEAD"); after != before {
		t.Fatalf("HEAD moved from %s to %s; fetch must not change the checkout", before, after)
	}
}

func TestFetchRejectsAFolderThatIsNotAClone(t *testing.T) {
	_, err := Fetch(context.Background(), t.TempDir(), "acme/api")
	if err == nil || !strings.Contains(err.Error(), "not a Git repository") {
		t.Fatalf("err = %v, want a not-a-Git-repository error", err)
	}
}

func TestFetchRejectsAMissingFolder(t *testing.T) {
	_, err := Fetch(context.Background(), filepath.Join(t.TempDir(), "missing"), "acme/api")
	if err == nil || !strings.Contains(err.Error(), "does not exist") {
		t.Fatalf("err = %v, want a does-not-exist error", err)
	}
}

func TestFetchRejectsACloneOfAnotherRepository(t *testing.T) {
	clone, _ := githubClone(t, "https://github.com/acme/web.git")

	_, err := Fetch(context.Background(), clone, "acme/api")
	if err == nil || !strings.Contains(err.Error(), "no remote points at github.com/acme/api") {
		t.Fatalf("err = %v, want a missing-remote error", err)
	}
}

func TestFetchReportsGitFailures(t *testing.T) {
	clone, _ := githubClone(t, "https://github.com/acme/api.git")
	runGit(t, clone, "config", "--unset", "url."+filepath.Join(filepath.Dir(clone), "remote.git")+".insteadOf")
	runGit(t, clone, "config", "url."+filepath.Join(filepath.Dir(clone), "gone.git")+".insteadOf", "https://github.com/acme/api.git")

	_, err := Fetch(context.Background(), clone, "acme/api")
	if err == nil || !strings.Contains(err.Error(), "git fetch origin") {
		t.Fatalf("err = %v, want the failing git fetch command", err)
	}
}

func TestFetchWithoutARepositoryUsesTheProjectsGitHubRemote(t *testing.T) {
	clone, seed := githubClone(t, "git@github.com:Acme/API.git")
	runGit(t, seed, "commit", "--quiet", "--allow-empty", "-m", "second")
	pushed := runGit(t, seed, "rev-parse", "HEAD")
	runGit(t, seed, "push", "--quiet", filepath.Join(filepath.Dir(seed), "remote.git"), "HEAD:refs/heads/main")

	result, err := Fetch(context.Background(), clone, "  ")
	if err != nil {
		t.Fatalf("fetch: %v", err)
	}
	if result.Repository != "Acme/API" || result.Remote != "origin" {
		t.Fatalf("result = %+v, want Acme/API as written in origin", result)
	}
	if got := runGit(t, clone, "rev-parse", "refs/remotes/origin/main"); got != pushed {
		t.Fatalf("origin/main = %s, want %s", got, pushed)
	}
}

func TestFetchWithoutARepositoryFailsWhenTheProjectHasNoGitHubRemote(t *testing.T) {
	clone, _ := githubClone(t, "https://gitlab.com/acme/api.git")

	_, err := Fetch(context.Background(), clone, "")
	if err == nil || !strings.Contains(err.Error(), "no GitHub remote") {
		t.Fatalf("err = %v, want a no-GitHub-remote error", err)
	}
}

func TestDetectGitHubRepositoryPrefersOrigin(t *testing.T) {
	clone, _ := githubClone(t, "https://github.com/acme/api.git")
	runGit(t, clone, "remote", "add", "fork", "git@github.com:someone/api.git")

	detected, err := DetectGitHubRepository(context.Background(), clone)
	if err != nil || detected.Repository != "acme/api" || detected.Remote != "origin" {
		t.Fatalf("detected = %+v, %v; want acme/api from origin", detected, err)
	}
}

func TestDetectGitHubRepositoryUsesTheOnlyGitHubRemote(t *testing.T) {
	clone, _ := githubClone(t, "https://gitlab.com/acme/api.git")
	runGit(t, clone, "remote", "add", "upstream", "git@github.com:acme/api.git")
	runGit(t, clone, "remote", "add", "mirror", "https://github.com/ACME/api")

	detected, err := DetectGitHubRepository(context.Background(), clone)
	if err != nil || detected.Repository != "ACME/api" || detected.Remote != "mirror" {
		t.Fatalf("detected = %+v, %v; want acme/api from the first matching remote", detected, err)
	}
}

func TestDetectGitHubRepositoryRejectsAmbiguousRemotes(t *testing.T) {
	clone, _ := githubClone(t, "https://gitlab.com/acme/api.git")
	runGit(t, clone, "remote", "add", "one", "git@github.com:acme/api.git")
	runGit(t, clone, "remote", "add", "two", "git@github.com:acme/web.git")

	_, err := DetectGitHubRepository(context.Background(), clone)
	if err == nil || !strings.Contains(err.Error(), "several GitHub repositories") {
		t.Fatalf("err = %v, want an ambiguity error", err)
	}
}

func TestDetectGitHubRepositoryRejectsAFolderThatIsNotAClone(t *testing.T) {
	_, err := DetectGitHubRepository(context.Background(), t.TempDir())
	if err == nil || !strings.Contains(err.Error(), "not a Git repository") {
		t.Fatalf("err = %v, want a not-a-Git-repository error", err)
	}
}

func TestParseGitHubRepository(t *testing.T) {
	valid := map[string]string{
		"acme/api":                           "acme/api",
		" Acme/API ":                         "Acme/API",
		"github.com/acme/api":                "acme/api",
		"https://github.com/acme/api":        "acme/api",
		"https://github.com/acme/api.git":    "acme/api",
		"https://github.com/acme/api/":       "acme/api",
		"https://user@github.com/acme/api":   "acme/api",
		"git@github.com:acme/api.git":        "acme/api",
		"ssh://git@github.com/acme/api.git":  "acme/api",
		"ssh://git@github.com:22/acme/api":   "acme/api",
		"https://www.github.com/acme/my.api": "acme/my.api",
	}
	for input, want := range valid {
		got, err := ParseGitHubRepository(input)
		if err != nil || got != want {
			t.Errorf("ParseGitHubRepository(%q) = %q, %v; want %q", input, got, err, want)
		}
	}
	invalid := []string{
		"",
		"acme",
		"acme/api/extra",
		"https://gitlab.com/acme/api",
		"git@gitlab.com:acme/api.git",
		"/tmp/acme/api",
		"acme/../api",
	}
	for _, input := range invalid {
		if got, err := ParseGitHubRepository(input); err == nil {
			t.Errorf("ParseGitHubRepository(%q) = %q, want an error", input, got)
		}
	}
}

func TestCleanEnvironmentDropsRepositoryVariables(t *testing.T) {
	cleaned := CleanEnvironment([]string{"GIT_DIR=/x", "HOME=/home/a", "GIT_WORK_TREE=/y", "GIT_AUTHOR_NAME=a"})
	if strings.Join(cleaned, ",") != "HOME=/home/a,GIT_AUTHOR_NAME=a" {
		t.Fatalf("cleaned = %v", cleaned)
	}
}

func TestGitCommandsAreWrittenToTheContextLog(t *testing.T) {
	clone, seed := githubClone(t, "https://github.com/acme/api.git")
	runGit(t, seed, "commit", "--quiet", "--allow-empty", "-m", "second")
	runGit(t, seed, "push", "--quiet", filepath.Join(filepath.Dir(seed), "remote.git"), "HEAD:refs/heads/main")
	var log strings.Builder

	if _, err := Fetch(WithLog(context.Background(), &log), clone, "acme/api"); err != nil {
		t.Fatalf("fetch: %v", err)
	}
	if !strings.Contains(log.String(), "$ git fetch origin\n") || !strings.Contains(log.String(), "main       -> origin/main") {
		t.Fatalf("log = %q, want the fetch command and its output", log.String())
	}
}

func TestAFailingGitCommandLogsItsExitStatus(t *testing.T) {
	var log strings.Builder

	_, _ = DetectGitHubRepository(WithLog(context.Background(), &log), t.TempDir())

	if !strings.Contains(log.String(), "$ git rev-parse --is-inside-work-tree\n") || !strings.Contains(log.String(), "exit status 128") {
		t.Fatalf("log = %q", log.String())
	}
}
