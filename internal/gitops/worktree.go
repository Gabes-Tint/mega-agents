package gitops

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// Workspace is a prepared worktree: the resource an agent implements in. It
// carries the repository identity, where the checkout lives, the branch it
// works on, and the base that branch is measured against.
type Workspace struct {
	Repository   string `json:"repository"`
	Remote       string `json:"remote,omitempty"`
	Path         string `json:"path"`
	Branch       string `json:"branch"`
	Base         string `json:"base"`
	BaseRevision string `json:"baseRevision"`
}

// WorktreeRequest describes the worktree to prepare from the clone in Dir.
// An empty Repository is detected from the clone's GitHub remote, an empty
// Base is that remote's default branch, and an empty Path is a folder under
// the Mega Agents home.
type WorktreeRequest struct {
	Dir        string
	Repository string
	Branch     string
	Base       string
	Path       string
}

// CreateWorktree adds a Git worktree for the branch, creating the branch
// from the base when it does not exist yet. A rerun that finds the branch's
// worktree already at the path reuses it, so work in progress survives.
// The project's own checkout is never switched.
func CreateWorktree(ctx context.Context, request WorktreeRequest) (Workspace, error) {
	detected, err := resolveRemote(ctx, request.Dir, request.Repository)
	if err != nil {
		return Workspace{}, err
	}
	branch := strings.TrimSpace(request.Branch)
	if branch == "" {
		return Workspace{}, errors.New("set the branch the worktree works on")
	}
	if _, err := git(ctx, request.Dir, "check-ref-format", "--branch", branch); err != nil {
		return Workspace{}, fmt.Errorf("branch %q is not a valid branch name", branch)
	}
	base, err := resolveBase(ctx, request.Dir, detected.Remote, strings.TrimSpace(request.Base))
	if err != nil {
		return Workspace{}, err
	}
	revision, err := commitOf(ctx, request.Dir, base)
	if err != nil {
		return Workspace{}, err
	}
	path := request.Path
	if path == "" {
		if path, err = defaultWorktreePath(detected.Repository, branch); err != nil {
			return Workspace{}, err
		}
	}
	workspace := Workspace{
		Repository: detected.Repository, Remote: detected.Remote, Path: path,
		Branch: branch, Base: base, BaseRevision: revision,
	}
	reuse, err := existingWorktree(ctx, request.Dir, path, branch)
	if err != nil || reuse {
		return workspace, err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return Workspace{}, fmt.Errorf("cannot create the folder for worktree %s: %w", path, err)
	}
	args := []string{"worktree", "add", "--quiet", "--no-track", "-b", branch, path, revision}
	if _, err := git(ctx, request.Dir, "show-ref", "--verify", "--quiet", "refs/heads/"+branch); err == nil {
		args = []string{"worktree", "add", "--quiet", path, branch}
	}
	if _, err := git(ctx, request.Dir, args...); err != nil {
		return Workspace{}, fmt.Errorf("git worktree add %s failed: %w", path, err)
	}
	return workspace, nil
}

// resolveBase returns the explicit base, or the remote's default branch as
// recorded by the last clone or fetch.
func resolveBase(ctx context.Context, dir string, remote string, base string) (string, error) {
	if base != "" {
		return base, nil
	}
	if head, err := git(ctx, dir, "symbolic-ref", "--quiet", "--short", "refs/remotes/"+remote+"/HEAD"); err == nil {
		return head, nil
	}
	for _, candidate := range []string{"main", "master"} {
		ref := remote + "/" + candidate
		if _, err := git(ctx, dir, "rev-parse", "--verify", "--quiet", "refs/remotes/"+ref); err == nil {
			return ref, nil
		}
	}
	return "", fmt.Errorf("remote %s has no default branch in %s; set the base the worktree branches from", remote, dir)
}

func commitOf(ctx context.Context, dir string, ref string) (string, error) {
	revision, err := git(ctx, dir, "rev-parse", "--verify", "--quiet", ref+"^{commit}")
	if err != nil {
		return "", fmt.Errorf("base %q does not name a commit in %s", ref, dir)
	}
	return revision, nil
}

// defaultWorktreePath keeps worktrees outside the project folder, so they
// never show up as untracked files in the project's own checkout.
func defaultWorktreePath(repository string, branch string) (string, error) {
	home := os.Getenv("MEGA_AGENTS_HOME")
	if home == "" {
		userHome, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("cannot resolve a folder for the worktree; set its path: %w", err)
		}
		home = filepath.Join(userHome, ".mega-agents")
	}
	owner, name, _ := strings.Cut(repository, "/")
	return filepath.Join(home, "worktrees", owner, name, strings.ReplaceAll(branch, "/", "-")), nil
}

// existingWorktree reports whether path already holds this repository's
// worktree for the branch. Any other content at the path is an error.
func existingWorktree(ctx context.Context, dir string, path string, branch string) (bool, error) {
	if _, err := os.Stat(path); errors.Is(err, fs.ErrNotExist) {
		return false, nil
	}
	listing, err := git(ctx, dir, "worktree", "list", "--porcelain")
	if err != nil {
		return false, fmt.Errorf("git worktree list failed: %w", err)
	}
	want, err := filepath.EvalSymlinks(path)
	if err != nil {
		want = path
	}
	for _, entry := range strings.Split(listing, "\n\n") {
		var entryPath, entryBranch string
		for _, line := range strings.Split(entry, "\n") {
			if value, ok := strings.CutPrefix(line, "worktree "); ok {
				entryPath = value
			}
			if value, ok := strings.CutPrefix(line, "branch refs/heads/"); ok {
				entryBranch = value
			}
		}
		if resolved, err := filepath.EvalSymlinks(entryPath); err == nil && resolved == want && entryBranch == branch {
			return true, nil
		}
	}
	return false, fmt.Errorf("worktree path %s already exists and is not the worktree of branch %s", path, branch)
}

// Rebase replays the workspace branch onto a new base, the workspace's own
// base when onto is empty. A rebase that stops on conflicts is aborted, so
// the branch is left exactly as it was.
func Rebase(ctx context.Context, workspace Workspace, onto string) (Workspace, error) {
	if info, err := os.Stat(workspace.Path); err != nil || !info.IsDir() {
		return Workspace{}, fmt.Errorf("workspace %s does not exist", workspace.Path)
	}
	onto = strings.TrimSpace(onto)
	if onto == "" {
		onto = workspace.Base
	}
	status, err := git(ctx, workspace.Path, "status", "--porcelain")
	if err != nil {
		return Workspace{}, fmt.Errorf("git status in %s failed: %w", workspace.Path, err)
	}
	if status != "" {
		return Workspace{}, fmt.Errorf("workspace %s has uncommitted changes; commit or discard them before rebasing", workspace.Path)
	}
	revision, err := commitOf(ctx, workspace.Path, onto)
	if err != nil {
		return Workspace{}, err
	}
	if _, err := git(ctx, workspace.Path, "rebase", revision); err != nil {
		_, _ = git(ctx, workspace.Path, "rebase", "--abort")
		return Workspace{}, fmt.Errorf("rebase of %s onto %s stopped on conflicts and was aborted: %w", workspace.Branch, onto, err)
	}
	workspace.Base = onto
	workspace.BaseRevision = revision
	return workspace, nil
}
