package gitops

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func changedWorkspace(t *testing.T) (Workspace, string) {
	t.Helper()
	clone, seed := githubClone(t, "https://github.com/acme/api.git")
	runGit(t, clone, "config", "user.name", "Test")
	runGit(t, clone, "config", "user.email", "test@example.com")
	workspace, err := CreateWorktree(context.Background(), WorktreeRequest{
		Dir: clone, Branch: "feature/login", Path: filepath.Join(t.TempDir(), "login"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workspace.Path, "login.go"), []byte("package login\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	return workspace, seed
}

func TestCommitRecordsEveryChangeInTheWorkspace(t *testing.T) {
	workspace, _ := changedWorkspace(t)

	commit, err := Commit(context.Background(), workspace, "Add login")

	if err != nil {
		t.Fatalf("commit: %v", err)
	}
	if head := runGit(t, workspace.Path, "rev-parse", "HEAD"); commit != head {
		t.Fatalf("commit = %s, HEAD = %s", commit, head)
	}
	if subject := runGit(t, workspace.Path, "log", "-1", "--format=%s"); subject != "Add login" {
		t.Fatalf("subject = %q", subject)
	}
	if status := runGit(t, workspace.Path, "status", "--porcelain"); status != "" {
		t.Fatalf("status = %q, want a clean tree", status)
	}
}

func TestCommitRefusesAWorkspaceWithoutChanges(t *testing.T) {
	workspace, _ := changedWorkspace(t)
	if _, err := Commit(context.Background(), workspace, "Add login"); err != nil {
		t.Fatal(err)
	}

	_, err := Commit(context.Background(), workspace, "Again")

	if err == nil || !strings.Contains(err.Error(), "has no changes to commit") {
		t.Fatalf("err = %v", err)
	}
	if _, err := Commit(context.Background(), workspace, " "); err == nil || !strings.Contains(err.Error(), "write the commit message") {
		t.Fatalf("err = %v", err)
	}
}

func TestPushPublishesTheBranchToItsRemote(t *testing.T) {
	workspace, seed := changedWorkspace(t)
	commit, err := Commit(context.Background(), workspace, "Add login")
	if err != nil {
		t.Fatal(err)
	}

	if err := Push(context.Background(), workspace); err != nil {
		t.Fatalf("push: %v", err)
	}
	bare := filepath.Join(filepath.Dir(seed), "remote.git")
	if remote := runGit(t, bare, "rev-parse", "refs/heads/feature/login"); remote != commit {
		t.Fatalf("remote branch = %s, want %s", remote, commit)
	}
}

func TestPushExplainsARejectedPush(t *testing.T) {
	workspace, _ := changedWorkspace(t)
	workspace.Remote = "nowhere"

	if err := Push(context.Background(), workspace); err == nil || !strings.Contains(err.Error(), "git push nowhere feature/login failed") {
		t.Fatalf("err = %v", err)
	}
}

// fakeGH installs a gh that records its arguments and prints output.
func fakeGH(t *testing.T, output string, exit int) string {
	t.Helper()
	bin := t.TempDir()
	script := "#!/bin/sh\nprintf '%s\\n' \"$@\" > \"$(dirname \"$0\")/args\"\ncat <<'OUT'\n" + output + "\nOUT\nexit " + string(rune('0'+exit)) + "\n"
	if err := os.WriteFile(filepath.Join(bin, "gh"), []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	return bin
}

func TestOpenPullRequestUsesTheWorkspaceBranchAndBase(t *testing.T) {
	bin := fakeGH(t, "https://github.com/acme/api/pull/42", 0)
	workspace := Workspace{Repository: "acme/api", Path: t.TempDir(), Branch: "feature/login", Base: "origin/main"}

	pull, err := OpenPullRequest(context.Background(), workspace, PullRequest{Title: "Add login", Body: "Closes #7"})

	if err != nil || pull.Number != 42 || pull.URL != "https://github.com/acme/api/pull/42" {
		t.Fatalf("pull = %+v, %v", pull, err)
	}
	args, _ := os.ReadFile(filepath.Join(bin, "args"))
	want := "pr\ncreate\n--repo\nacme/api\n--head\nfeature/login\n--base\nmain\n--title\nAdd login\n--body\nCloses #7\n"
	if string(args) != want {
		t.Fatalf("args =\n%s\nwant\n%s", args, want)
	}
}

func TestOpenPullRequestReportsGHFailures(t *testing.T) {
	fakeGH(t, "a pull request for branch feature/login already exists", 1)
	workspace := Workspace{Repository: "acme/api", Path: t.TempDir(), Branch: "feature/login", Base: "main"}

	_, err := OpenPullRequest(context.Background(), workspace, PullRequest{Title: "x"})

	if err == nil || !strings.Contains(err.Error(), "gh pr create failed") || !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("err = %v", err)
	}
	if _, err := OpenPullRequest(context.Background(), workspace, PullRequest{}); err == nil || !strings.Contains(err.Error(), "write the pull request title") {
		t.Fatalf("err = %v", err)
	}
}

func TestReadIssueReturnsItsFields(t *testing.T) {
	bin := fakeGH(t, `{"number":7,"title":"Login","body":"Users need to log in","url":"https://github.com/acme/api/issues/7","labels":[{"name":"story"}],"comments":[]}`, 0)

	issue, err := ReadIssue(context.Background(), "acme/api", 7)

	if err != nil || issue["title"] != "Login" || issue["number"] != 7.0 {
		t.Fatalf("issue = %+v, %v", issue, err)
	}
	args, _ := os.ReadFile(filepath.Join(bin, "args"))
	if !strings.HasPrefix(string(args), "issue\nview\n7\n--repo\nacme/api\n--json\nnumber,title,body,url,labels,comments\n") {
		t.Fatalf("args = %q", args)
	}
	fakeGH(t, "issue not found", 1)
	if _, err := ReadIssue(context.Background(), "acme/api", 9); err == nil || !strings.Contains(err.Error(), "gh issue view 9 failed: issue not found") {
		t.Fatalf("err = %v", err)
	}
}

func TestFetchesAndPushesOfOneRepositoryDoNotCollide(t *testing.T) {
	clone, seed := githubClone(t, "https://github.com/acme/api.git")
	runGit(t, clone, "config", "user.name", "Test")
	runGit(t, clone, "config", "user.email", "test@example.com")
	pushCommit(t, seed, "moved")
	var workspaces []Workspace
	for i := range 4 {
		workspace, err := CreateWorktree(context.Background(), WorktreeRequest{
			Dir: clone, Branch: "push-" + string(rune('a'+i)), Path: filepath.Join(t.TempDir(), "w"),
		})
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(workspace.Path, "f.txt"), []byte(workspace.Branch), 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := Commit(context.Background(), workspace, "change"); err != nil {
			t.Fatal(err)
		}
		workspaces = append(workspaces, workspace)
	}
	errs := make(chan error, 8)
	for _, workspace := range workspaces {
		go func() { errs <- Push(context.Background(), workspace) }()
		go func() {
			_, err := Fetch(context.Background(), clone, "acme/api")
			errs <- err
		}()
	}
	for range 8 {
		if err := <-errs; err != nil {
			t.Fatalf("concurrent git: %v", err)
		}
	}
}
