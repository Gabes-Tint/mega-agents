// Package gitops runs local Git operations on behalf of workflow blocks.
package gitops

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"net/url"
	"os"
	"os/exec"
	"regexp"
	"sort"
	"strings"
)

// FetchResult is the evidence of a completed fetch.
type FetchResult struct {
	Remote string `json:"remote"`
	Output string `json:"output"`
}

// Fetch runs git fetch in dir for the remote that points at the GitHub
// repository. It relies on credentials the machine already has (SSH keys or
// a credential helper) and never prompts, so a missing login fails instead of
// hanging the run. The checkout and local branches are left untouched.
func Fetch(ctx context.Context, dir string, repository string) (FetchResult, error) {
	want, err := ParseGitHubRepository(repository)
	if err != nil {
		return FetchResult{}, err
	}
	info, err := os.Stat(dir)
	if errors.Is(err, fs.ErrNotExist) {
		return FetchResult{}, fmt.Errorf("project path %s does not exist", dir)
	}
	if err != nil {
		return FetchResult{}, fmt.Errorf("project path %s is not readable: %w", dir, err)
	}
	if !info.IsDir() {
		return FetchResult{}, fmt.Errorf("project path %s is not a directory", dir)
	}
	if _, err := git(ctx, dir, "rev-parse", "--is-inside-work-tree"); err != nil {
		return FetchResult{}, fmt.Errorf("project path %s is not a Git repository", dir)
	}
	remote, err := matchingRemote(ctx, dir, want)
	if err != nil {
		return FetchResult{}, err
	}
	output, err := git(ctx, dir, "fetch", remote)
	if err != nil {
		return FetchResult{}, fmt.Errorf("git fetch %s failed: %w", remote, err)
	}
	return FetchResult{Remote: remote, Output: output}, nil
}

// matchingRemote finds the remote whose configured URL names the repository,
// preferring origin when several do. The raw config value is read so
// insteadOf rewrites do not hide which repository a remote represents.
func matchingRemote(ctx context.Context, dir string, want string) (string, error) {
	// git config exits 1 when no remote URL is configured; that is simply no match.
	output, _ := git(ctx, dir, "config", "--get-regexp", `^remote\..*\.url$`)
	var matches []string
	for _, line := range strings.Split(output, "\n") {
		key, value, ok := strings.Cut(strings.TrimSpace(line), " ")
		if !ok {
			continue
		}
		name := strings.TrimSuffix(strings.TrimPrefix(key, "remote."), ".url")
		if got, err := ParseGitHubRepository(value); err == nil && got == want {
			matches = append(matches, name)
		}
	}
	if len(matches) == 0 {
		return "", fmt.Errorf("no remote points at github.com/%s in %s", want, dir)
	}
	sort.Strings(matches)
	for _, name := range matches {
		if name == "origin" {
			return name, nil
		}
	}
	return matches[0], nil
}

var repositorySegment = regexp.MustCompile(`^[A-Za-z0-9._-]+$`)

// ParseGitHubRepository normalizes owner/name shorthand and HTTPS, SSH, and
// scp-style GitHub URLs to a lowercase owner/name, since GitHub treats
// repository names case-insensitively.
func ParseGitHubRepository(repository string) (string, error) {
	value := strings.TrimSpace(repository)
	invalid := fmt.Errorf("repository %q is not a GitHub owner/name or URL", repository)
	var host, path string
	switch {
	case strings.Contains(value, "://"):
		parsed, err := url.Parse(value)
		if err != nil {
			return "", invalid
		}
		host, path = parsed.Hostname(), parsed.Path
	case strings.HasPrefix(value, "git@"):
		host, path, _ = strings.Cut(strings.TrimPrefix(value, "git@"), ":")
	case strings.HasPrefix(strings.ToLower(value), "github.com/"):
		host, path = "github.com", value[len("github.com/"):]
	default:
		host, path = "github.com", value
	}
	host = strings.ToLower(host)
	if host != "github.com" && host != "www.github.com" {
		return "", invalid
	}
	path = strings.TrimSuffix(strings.Trim(path, "/"), ".git")
	segments := strings.Split(path, "/")
	if len(segments) != 2 {
		return "", invalid
	}
	for _, segment := range segments {
		if segment == "." || segment == ".." || !repositorySegment.MatchString(segment) {
			return "", invalid
		}
	}
	return strings.ToLower(path), nil
}

func git(ctx context.Context, dir string, args ...string) (string, error) {
	command := exec.CommandContext(ctx, "git", args...)
	command.Dir = dir
	command.Env = append(CleanEnvironment(os.Environ()), "GIT_TERMINAL_PROMPT=0")
	output, err := command.CombinedOutput()
	trimmed := strings.TrimSpace(string(output))
	if err != nil {
		return trimmed, fmt.Errorf("%w: %s", err, trimmed)
	}
	return trimmed, nil
}

// CleanEnvironment removes variables that pin Git to another repository, so
// commands run from inside a hook or a worktree still target their own dir.
func CleanEnvironment(environment []string) []string {
	repositoryVariables := map[string]bool{
		"GIT_ALTERNATE_OBJECT_DIRECTORIES": true,
		"GIT_COMMON_DIR":                   true,
		"GIT_DIR":                          true,
		"GIT_GRAFT_FILE":                   true,
		"GIT_INDEX_FILE":                   true,
		"GIT_OBJECT_DIRECTORY":             true,
		"GIT_PREFIX":                       true,
		"GIT_REPLACE_REF_BASE":             true,
		"GIT_SHALLOW_FILE":                 true,
		"GIT_WORK_TREE":                    true,
	}
	cleaned := make([]string, 0, len(environment))
	for _, variable := range environment {
		name, _, _ := strings.Cut(variable, "=")
		if !repositoryVariables[name] {
			cleaned = append(cleaned, variable)
		}
	}
	return cleaned
}
