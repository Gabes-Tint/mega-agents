package gitops

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// pushCommit adds an empty commit on the seed and pushes it to the bare
// remote's main branch, returning the new commit.
func pushCommit(t *testing.T, seed string, message string) string {
	t.Helper()
	runGit(t, seed, "commit", "--quiet", "--allow-empty", "-m", message)
	runGit(t, seed, "push", "--quiet", filepath.Join(filepath.Dir(seed), "remote.git"), "HEAD:refs/heads/main")
	return runGit(t, seed, "rev-parse", "HEAD")
}

func TestCreateWorktreeBranchesFromTheRemoteDefaultBranch(t *testing.T) {
	clone, seed := githubClone(t, "https://github.com/acme/api.git")
	pushed := pushCommit(t, seed, "second")
	runGit(t, clone, "fetch", "--quiet", "origin")
	target := filepath.Join(t.TempDir(), "login")

	workspace, err := CreateWorktree(context.Background(), WorktreeRequest{
		Dir: clone, Repository: "acme/api", Branch: "feature/login", Path: target,
	})
	if err != nil {
		t.Fatalf("create worktree: %v", err)
	}
	want := Workspace{
		Repository: "acme/api", Remote: "origin", Path: target, Branch: "feature/login",
		Base: "origin/main", BaseRevision: pushed,
	}
	if workspace != want {
		t.Fatalf("workspace = %+v, want %+v", workspace, want)
	}
	if got := runGit(t, target, "rev-parse", "HEAD"); got != pushed {
		t.Fatalf("worktree HEAD = %s, want the fetched base %s", got, pushed)
	}
	if got := runGit(t, target, "branch", "--show-current"); got != "feature/login" {
		t.Fatalf("worktree branch = %s", got)
	}
	if got := runGit(t, clone, "branch", "--show-current"); got != "main" {
		t.Fatalf("the project checkout moved to %s", got)
	}
}

func TestCreateWorktreeUsesAnExplicitBase(t *testing.T) {
	clone, seed := githubClone(t, "https://github.com/acme/api.git")
	first := runGit(t, clone, "rev-parse", "HEAD")
	pushCommit(t, seed, "second")
	runGit(t, clone, "fetch", "--quiet", "origin")

	workspace, err := CreateWorktree(context.Background(), WorktreeRequest{
		Dir: clone, Branch: "hotfix", Base: first, Path: filepath.Join(t.TempDir(), "hotfix"),
	})
	if err != nil {
		t.Fatalf("create worktree: %v", err)
	}
	if workspace.Base != first || workspace.BaseRevision != first || workspace.Repository != "acme/api" {
		t.Fatalf("workspace = %+v, want base %s from the detected repository", workspace, first)
	}
}

func TestCreateWorktreeDefaultsItsPathUnderTheMegaAgentsHome(t *testing.T) {
	clone, _ := githubClone(t, "https://github.com/acme/api.git")
	home := t.TempDir()
	t.Setenv("MEGA_AGENTS_HOME", home)

	workspace, err := CreateWorktree(context.Background(), WorktreeRequest{Dir: clone, Branch: "feature/login"})
	if err != nil {
		t.Fatalf("create worktree: %v", err)
	}
	want := filepath.Join(home, "worktrees", "acme", "api", "feature-login")
	if workspace.Path != want {
		t.Fatalf("path = %s, want %s", workspace.Path, want)
	}
	if _, err := os.Stat(filepath.Join(want, ".git")); err != nil {
		t.Fatalf("no worktree at %s: %v", want, err)
	}
}

func TestCreateWorktreeReusesItsOwnWorktreeOnARerun(t *testing.T) {
	clone, _ := githubClone(t, "https://github.com/acme/api.git")
	target := filepath.Join(t.TempDir(), "login")
	request := WorktreeRequest{Dir: clone, Branch: "feature/login", Path: target}
	if _, err := CreateWorktree(context.Background(), request); err != nil {
		t.Fatalf("first create: %v", err)
	}
	if err := os.WriteFile(filepath.Join(target, "work.txt"), []byte("in progress"), 0o600); err != nil {
		t.Fatal(err)
	}

	workspace, err := CreateWorktree(context.Background(), request)
	if err != nil {
		t.Fatalf("rerun: %v", err)
	}
	if workspace.Path != target {
		t.Fatalf("workspace = %+v", workspace)
	}
	if _, err := os.Stat(filepath.Join(target, "work.txt")); err != nil {
		t.Fatalf("rerun discarded the work in progress: %v", err)
	}
}

func TestCreateWorktreeChecksOutAnExistingBranch(t *testing.T) {
	clone, _ := githubClone(t, "https://github.com/acme/api.git")
	runGit(t, clone, "branch", "feature/login")
	runGit(t, clone, "commit", "--quiet", "--allow-empty", "-m", "on main only")
	branchTip := runGit(t, clone, "rev-parse", "feature/login")
	target := filepath.Join(t.TempDir(), "login")

	if _, err := CreateWorktree(context.Background(), WorktreeRequest{
		Dir: clone, Branch: "feature/login", Path: target,
	}); err != nil {
		t.Fatalf("create worktree: %v", err)
	}
	if got := runGit(t, target, "rev-parse", "HEAD"); got != branchTip {
		t.Fatalf("HEAD = %s, want the existing branch tip %s", got, branchTip)
	}
}

func TestCreateWorktreeRejectsInvalidRequests(t *testing.T) {
	clone, _ := githubClone(t, "https://github.com/acme/api.git")
	occupied := t.TempDir()
	if err := os.WriteFile(filepath.Join(occupied, "file"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	cases := map[string]struct {
		request WorktreeRequest
		want    string
	}{
		"missing branch": {
			request: WorktreeRequest{Dir: clone},
			want:    "set the branch",
		},
		"invalid branch": {
			request: WorktreeRequest{Dir: clone, Branch: "bad..name"},
			want:    `branch "bad..name" is not a valid branch name`,
		},
		"unknown base": {
			request: WorktreeRequest{Dir: clone, Branch: "x", Base: "origin/nope", Path: filepath.Join(t.TempDir(), "x")},
			want:    `base "origin/nope" does not name a commit`,
		},
		"occupied path": {
			request: WorktreeRequest{Dir: clone, Branch: "x", Path: occupied},
			want:    "is not the worktree of branch x",
		},
		"another repository": {
			request: WorktreeRequest{Dir: clone, Repository: "acme/web", Branch: "x"},
			want:    "no remote points at github.com/acme/web",
		},
		"not a clone": {
			request: WorktreeRequest{Dir: t.TempDir(), Branch: "x"},
			want:    "is not a Git repository",
		},
	}
	for name, testCase := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := CreateWorktree(context.Background(), testCase.request)
			if err == nil || !strings.Contains(err.Error(), testCase.want) {
				t.Fatalf("err = %v, want it to contain %q", err, testCase.want)
			}
		})
	}
}

func TestCreateWorktreeRequiresABaseWhenTheRemoteHasNoDefaultBranch(t *testing.T) {
	clone, _ := githubClone(t, "https://github.com/acme/api.git")
	runGit(t, clone, "remote", "set-head", "origin", "--delete")
	runGit(t, clone, "update-ref", "-d", "refs/remotes/origin/main")

	_, err := CreateWorktree(context.Background(), WorktreeRequest{
		Dir: clone, Branch: "x", Path: filepath.Join(t.TempDir(), "x"),
	})
	if err == nil || !strings.Contains(err.Error(), "set the base") {
		t.Fatalf("err = %v, want a set-the-base error", err)
	}
}

// branchWorkspace returns a worktree on feature/login holding one commit of
// its own, created before the remote's main moved ahead.
func branchWorkspace(t *testing.T) (Workspace, string) {
	t.Helper()
	clone, seed := githubClone(t, "https://github.com/acme/api.git")
	// Rebase replays commits, so the repository needs a committer identity
	// even where the machine has no global one.
	runGit(t, clone, "config", "user.name", "Test")
	runGit(t, clone, "config", "user.email", "test@example.com")
	workspace, err := CreateWorktree(context.Background(), WorktreeRequest{
		Dir: clone, Branch: "feature/login", Path: filepath.Join(t.TempDir(), "login"),
	})
	if err != nil {
		t.Fatalf("create worktree: %v", err)
	}
	if err := os.WriteFile(filepath.Join(workspace.Path, "login.txt"), []byte("login"), 0o600); err != nil {
		t.Fatal(err)
	}
	runGit(t, workspace.Path, "add", "login.txt")
	runGit(t, workspace.Path, "commit", "--quiet", "-m", "login")
	return workspace, seed
}

func TestRebaseReplaysTheBranchOntoTheMovedBase(t *testing.T) {
	workspace, seed := branchWorkspace(t)
	moved := pushCommit(t, seed, "main moved")
	runGit(t, workspace.Path, "fetch", "--quiet", "origin")

	rebased, err := Rebase(context.Background(), workspace, "")
	if err != nil {
		t.Fatalf("rebase: %v", err)
	}
	if rebased.BaseRevision != moved || rebased.Base != "origin/main" || rebased.Path != workspace.Path {
		t.Fatalf("rebased = %+v, want base revision %s", rebased, moved)
	}
	if got := runGit(t, workspace.Path, "rev-parse", "HEAD~1"); got != moved {
		t.Fatalf("HEAD~1 = %s, want the moved base %s", got, moved)
	}
	if got := runGit(t, workspace.Path, "log", "-1", "--format=%s"); got != "login" {
		t.Fatalf("HEAD subject = %q, want the branch's own commit", got)
	}
}

func TestRebaseOntoAnExplicitRevision(t *testing.T) {
	workspace, seed := branchWorkspace(t)
	moved := pushCommit(t, seed, "main moved")
	runGit(t, workspace.Path, "fetch", "--quiet", "origin")

	rebased, err := Rebase(context.Background(), workspace, moved)
	if err != nil {
		t.Fatalf("rebase: %v", err)
	}
	if rebased.Base != moved || rebased.BaseRevision != moved {
		t.Fatalf("rebased = %+v", rebased)
	}
}

func TestRebaseRefusesUncommittedChanges(t *testing.T) {
	workspace, _ := branchWorkspace(t)
	if err := os.WriteFile(filepath.Join(workspace.Path, "login.txt"), []byte("edited"), 0o600); err != nil {
		t.Fatal(err)
	}

	_, err := Rebase(context.Background(), workspace, "")
	if err == nil || !strings.Contains(err.Error(), "has uncommitted changes") {
		t.Fatalf("err = %v, want an uncommitted-changes error", err)
	}
}

func TestRebaseAbortsOnConflicts(t *testing.T) {
	workspace, seed := branchWorkspace(t)
	if err := os.WriteFile(filepath.Join(seed, "login.txt"), []byte("conflicting"), 0o600); err != nil {
		t.Fatal(err)
	}
	runGit(t, seed, "add", "login.txt")
	pushCommit(t, seed, "conflict")
	runGit(t, workspace.Path, "fetch", "--quiet", "origin")
	before := runGit(t, workspace.Path, "rev-parse", "HEAD")

	_, err := Rebase(context.Background(), workspace, "")
	if err == nil || !strings.Contains(err.Error(), "was aborted") {
		t.Fatalf("err = %v, want an aborted-rebase error", err)
	}
	if got := runGit(t, workspace.Path, "rev-parse", "HEAD"); got != before {
		t.Fatalf("HEAD = %s, want the branch restored to %s", got, before)
	}
	if status := runGit(t, workspace.Path, "status", "--porcelain"); status != "" {
		t.Fatalf("worktree left dirty: %q", status)
	}
}

func TestRebaseRejectsAMissingWorkspace(t *testing.T) {
	_, err := Rebase(context.Background(), Workspace{Path: filepath.Join(t.TempDir(), "gone"), Base: "origin/main"}, "")
	if err == nil || !strings.Contains(err.Error(), "does not exist") {
		t.Fatalf("err = %v, want a missing-workspace error", err)
	}
}
