package gitops

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
)

// PullRequest is what to open, and after OpenPullRequest, what was opened.
type PullRequest struct {
	Title  string `json:"title,omitempty"`
	Body   string `json:"body,omitempty"`
	Base   string `json:"base,omitempty"`
	Number int    `json:"number,omitempty"`
	URL    string `json:"url,omitempty"`
}

// Commit stages every change in the workspace and commits it, returning the
// new commit. A workspace with nothing to commit is an error: the step
// before it was expected to change something.
func Commit(ctx context.Context, workspace Workspace, message string) (string, error) {
	if strings.TrimSpace(message) == "" {
		return "", errors.New("write the commit message")
	}
	if _, err := git(ctx, workspace.Path, "add", "--all"); err != nil {
		return "", fmt.Errorf("git add in %s failed: %w", workspace.Path, err)
	}
	if _, err := git(ctx, workspace.Path, "diff", "--cached", "--quiet"); err == nil {
		return "", fmt.Errorf("workspace %s has no changes to commit", workspace.Path)
	}
	if _, err := git(ctx, workspace.Path, "commit", "--quiet", "--no-verify", "-m", message); err != nil {
		return "", fmt.Errorf("git commit in %s failed: %w", workspace.Path, err)
	}
	return git(ctx, workspace.Path, "rev-parse", "HEAD")
}

// Push publishes the workspace branch to the remote it came from, with the
// machine's existing credentials.
func Push(ctx context.Context, workspace Workspace) error {
	remote := workspace.Remote
	if remote == "" {
		remote = "origin"
	}
	defer lockRepository(ctx, workspace.Path)()
	if _, err := git(ctx, workspace.Path, "push", "--set-upstream", remote, "HEAD:refs/heads/"+workspace.Branch); err != nil {
		return fmt.Errorf("git push %s %s failed: %w", remote, workspace.Branch, err)
	}
	return nil
}

var pullNumber = regexp.MustCompile(`/pull/(\d+)`)

// OpenPullRequest opens a pull request from the workspace branch with the
// GitHub CLI, into the given base or the branch the workspace was cut from.
func OpenPullRequest(ctx context.Context, workspace Workspace, request PullRequest) (PullRequest, error) {
	if strings.TrimSpace(request.Title) == "" {
		return PullRequest{}, errors.New("write the pull request title")
	}
	base := request.Base
	if base == "" {
		base = workspace.Base
		if remote, branch, remoteBranch := strings.Cut(base, "/"); remoteBranch && remote == remoteOf(workspace) {
			base = branch
		}
	}
	output, err := gh(ctx, workspace.Path, "pr", "create", "--repo", workspace.Repository,
		"--head", workspace.Branch, "--base", base, "--title", request.Title, "--body", request.Body)
	if err != nil {
		return PullRequest{}, fmt.Errorf("gh pr create failed: %w", err)
	}
	request.Base = base
	for _, line := range strings.Split(output, "\n") {
		if match := pullNumber.FindStringSubmatch(line); match != nil {
			request.URL = strings.TrimSpace(line)
			request.Number, _ = strconv.Atoi(match[1])
		}
	}
	return request, nil
}

func remoteOf(workspace Workspace) string {
	if workspace.Remote == "" {
		return "origin"
	}
	return workspace.Remote
}

// ReadIssue fetches an issue's number, title, body, URL, labels and
// comments with the GitHub CLI.
func ReadIssue(ctx context.Context, repository string, number int) (map[string]any, error) {
	output, err := gh(ctx, "", "issue", "view", strconv.Itoa(number), "--repo", repository,
		"--json", "number,title,body,url,labels,comments")
	if err != nil {
		return nil, fmt.Errorf("gh issue view %d failed: %w", number, err)
	}
	var issue map[string]any
	if err := json.Unmarshal([]byte(output), &issue); err != nil {
		return nil, fmt.Errorf("gh issue view %d printed no issue: %w", number, err)
	}
	return issue, nil
}

// IgnoredLabel returns the first of an issue's labels that matches one of
// the labels to ignore, compared without case as GitHub does, or "".
func IgnoredLabel(issue map[string]any, ignore []string) string {
	labels, _ := issue["labels"].([]any)
	for _, entry := range labels {
		label, _ := entry.(map[string]any)
		name, _ := label["name"].(string)
		for _, ignored := range ignore {
			if ignored = strings.TrimSpace(ignored); ignored != "" && strings.EqualFold(name, ignored) {
				return name
			}
		}
	}
	return ""
}

func gh(ctx context.Context, dir string, args ...string) (string, error) {
	command := exec.CommandContext(ctx, "gh", args...)
	command.Dir = dir
	command.Env = append(CleanEnvironment(os.Environ()), "GH_PROMPT_DISABLED=1", "GIT_TERMINAL_PROMPT=0")
	output, err := command.CombinedOutput()
	trimmed := strings.TrimSpace(string(output))
	if log, ok := ctx.Value(logKey{}).(io.Writer); ok {
		fmt.Fprintf(log, "$ gh %s\n", describeGH(args))
		if trimmed != "" {
			fmt.Fprintln(log, trimmed)
		}
	}
	if err != nil {
		if trimmed == "" {
			return "", err
		}
		return "", errors.New(trimmed)
	}
	return trimmed, nil
}

// describeGH keeps a long pull request body out of the log line.
func describeGH(args []string) string {
	shown := make([]string, len(args))
	for i, arg := range args {
		shown[i] = arg
		if i > 0 && args[i-1] == "--body" && len(arg) > 80 {
			shown[i] = fmt.Sprintf("<body: %d chars>", len(arg))
		}
	}
	return strings.Join(shown, " ")
}
