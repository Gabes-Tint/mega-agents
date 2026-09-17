// Package gitops runs local Git operations on behalf of workflow blocks.
package gitops

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/url"
	"os"
	"os/exec"
	"regexp"
	"slices"
	"strings"
)

// FetchResult is the evidence of a completed fetch.
type FetchResult struct {
	Repository string `json:"repository"`
	Remote     string `json:"remote"`
	Output     string `json:"output"`
}

// DetectedRepository is the GitHub repository a clone's remotes point at.
type DetectedRepository struct {
	Repository string `json:"repository"`
	Remote     string `json:"remote"`
}

// Fetch runs git fetch in dir for the remote that points at the GitHub
// repository. An empty repository falls back to the one the clone's remotes
// point at. It relies on credentials the machine already has (SSH keys or a
// credential helper) and never prompts, so a missing login fails instead of
// hanging the run. The checkout and local branches are left untouched.
func Fetch(ctx context.Context, dir string, repository string) (FetchResult, error) {
	detected, err := resolveRemote(ctx, dir, repository)
	if err != nil {
		return FetchResult{}, err
	}
	defer lockRepository(ctx, dir)()
	output, err := git(ctx, dir, "fetch", detected.Remote)
	if err != nil {
		return FetchResult{}, fmt.Errorf("git fetch %s failed: %w", detected.Remote, err)
	}
	return FetchResult{Repository: detected.Repository, Remote: detected.Remote, Output: output}, nil
}

// resolveRemote finds the remote of the clone in dir that points at the
// repository, or at the clone's own GitHub repository when none is named.
func resolveRemote(ctx context.Context, dir string, repository string) (DetectedRepository, error) {
	if strings.TrimSpace(repository) == "" {
		return DetectGitHubRepository(ctx, dir)
	}
	want, err := ParseGitHubRepository(repository)
	if err != nil {
		return DetectedRepository{}, err
	}
	if err := requireRepositoryDir(ctx, dir); err != nil {
		return DetectedRepository{}, err
	}
	return matchingRemote(ctx, dir, want)
}

// DetectGitHubRepository reports the GitHub repository of the clone in dir:
// origin's when origin points at GitHub, otherwise the single repository the
// GitHub remotes share. Remotes naming different repositories are ambiguous.
func DetectGitHubRepository(ctx context.Context, dir string) (DetectedRepository, error) {
	if err := requireRepositoryDir(ctx, dir); err != nil {
		return DetectedRepository{}, err
	}
	remotes := githubRemotes(ctx, dir)
	if repository, ok := remotes["origin"]; ok {
		return DetectedRepository{Repository: repository, Remote: "origin"}, nil
	}
	names := make([]string, 0, len(remotes))
	for name := range remotes {
		names = append(names, name)
	}
	slices.Sort(names)
	if len(names) == 0 {
		return DetectedRepository{}, fmt.Errorf(
			"no GitHub remote in %s; set the repository on the GitHub block", dir,
		)
	}
	first := DetectedRepository{Repository: remotes[names[0]], Remote: names[0]}
	for _, name := range names[1:] {
		if !strings.EqualFold(remotes[name], first.Repository) {
			return DetectedRepository{}, fmt.Errorf(
				"remotes in %s point at several GitHub repositories; set the repository on the GitHub block", dir,
			)
		}
	}
	return first, nil
}

func requireRepositoryDir(ctx context.Context, dir string) error {
	info, err := os.Stat(dir)
	if errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("project path %s does not exist", dir)
	}
	if err != nil {
		return fmt.Errorf("project path %s is not readable: %w", dir, err)
	}
	if !info.IsDir() {
		return fmt.Errorf("project path %s is not a directory", dir)
	}
	if _, err := git(ctx, dir, "rev-parse", "--is-inside-work-tree"); err != nil {
		return fmt.Errorf("project path %s is not a Git repository", dir)
	}
	return nil
}

// githubRemotes maps each remote that points at GitHub to its normalized
// repository. The raw config value is read so insteadOf rewrites do not hide
// which repository a remote represents.
func githubRemotes(ctx context.Context, dir string) map[string]string {
	// git config exits 1 when no remote URL is configured; that is simply no remote.
	output, _ := git(ctx, dir, "config", "--get-regexp", `^remote\..*\.url$`)
	remotes := map[string]string{}
	for _, line := range strings.Split(output, "\n") {
		key, value, ok := strings.Cut(strings.TrimSpace(line), " ")
		if !ok {
			continue
		}
		name := strings.TrimSuffix(strings.TrimPrefix(key, "remote."), ".url")
		if repository, err := ParseGitHubRepository(value); err == nil {
			remotes[name] = repository
		}
	}
	return remotes
}

// matchingRemote finds the remote that names the repository, preferring
// origin when several do.
func matchingRemote(ctx context.Context, dir string, want string) (DetectedRepository, error) {
	var matches []string
	for name, repository := range githubRemotes(ctx, dir) {
		if strings.EqualFold(repository, want) {
			matches = append(matches, name)
		}
	}
	if len(matches) == 0 {
		return DetectedRepository{}, fmt.Errorf("no remote points at github.com/%s in %s", want, dir)
	}
	slices.Sort(matches)
	remote := matches[0]
	if slices.Contains(matches, "origin") {
		remote = "origin"
	}
	return DetectedRepository{Repository: want, Remote: remote}, nil
}

var repositorySegment = regexp.MustCompile(`^[A-Za-z0-9._-]+$`)

// ParseGitHubRepository reduces owner/name shorthand and HTTPS, SSH, and
// scp-style GitHub URLs to owner/name, keeping the casing as written. GitHub
// treats names case-insensitively, so compare results with strings.EqualFold.
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
	return path, nil
}

type logKey struct{}

// WithLog returns a context whose Git commands, with their output and exit
// status, are written to log.
func WithLog(ctx context.Context, log io.Writer) context.Context {
	return context.WithValue(ctx, logKey{}, log)
}

func git(ctx context.Context, dir string, args ...string) (string, error) {
	command := exec.CommandContext(ctx, "git", args...)
	command.Dir = dir
	command.Env = append(CleanEnvironment(os.Environ()), "GIT_TERMINAL_PROMPT=0")
	output, err := command.CombinedOutput()
	trimmed := strings.TrimSpace(string(output))
	if log, ok := ctx.Value(logKey{}).(io.Writer); ok {
		fmt.Fprintf(log, "$ git %s\n", strings.Join(args, " "))
		if trimmed != "" {
			fmt.Fprintln(log, trimmed)
		}
		if err != nil {
			fmt.Fprintf(log, "(%s)\n", err)
		}
	}
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
