package agents

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"
)

// States of a backend's health check.
const (
	HealthChecking    = "checking"
	HealthAvailable   = "available"
	HealthUnavailable = "unavailable"
)

// HealthCheckPrompt is the one tiny turn a health check spends.
const HealthCheckPrompt = "Reply with exactly: ACK"

// Labels names each backend for people.
var Labels = map[string]string{
	"claude":   "Claude Code",
	"codex":    "Codex",
	"grok":     "Grok",
	"opencode": "OpenCode",
}

// HealthCheckModels is the cheapest model each check runs on; empty keeps
// the CLI's default.
var HealthCheckModels = map[string]string{
	"claude":   "haiku",
	"codex":    "",
	"grok":     "",
	"opencode": "opencode-go/glm-5.3-flash",
}

const reasonLimit = 120

// Health is what the last check learned about one backend. Reason says why
// it is unavailable; Acknowledged records a reply of exactly ACK.
type Health struct {
	Backend      string     `json:"backend"`
	Label        string     `json:"label"`
	State        string     `json:"state"`
	Reason       string     `json:"reason,omitempty"`
	ReplyMs      int64      `json:"replyMs,omitempty"`
	CheckedAt    *time.Time `json:"checkedAt,omitempty"`
	Acknowledged bool       `json:"acknowledged"`
}

// HealthChecker sends each backend one tiny prompt and remembers whether it
// answered. A check runs in the background; Snapshot reads its progress.
type HealthChecker struct {
	// Backends to check, in order; all of them by default.
	Backends []string
	Timeout  time.Duration
	// Disabled reports every backend as turned off and runs nothing.
	Disabled bool

	mu      sync.Mutex
	health  map[string]Health
	running sync.WaitGroup
	pending int
}

func NewHealthChecker() *HealthChecker {
	return &HealthChecker{Backends: Names, Timeout: 60 * time.Second}
}

// Start checks every backend in parallel unless a check is already running.
// A backend whose CLI is not on PATH is marked at once without running it.
func (c *HealthChecker) Start() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.Disabled || c.pending > 0 {
		return
	}
	if c.health == nil {
		c.health = map[string]Health{}
	}
	for _, name := range c.Backends {
		backend, err := Lookup(name)
		if err != nil {
			continue
		}
		if _, err := exec.LookPath(backend.Command(Turn{}).Args[0]); err != nil {
			c.health[name] = c.finished(name, time.Now(), "not installed")
			continue
		}
		previous := c.health[name]
		c.health[name] = Health{Backend: name, Label: Labels[name], State: HealthChecking, CheckedAt: previous.CheckedAt}
		c.pending++
		c.running.Add(1)
		go func() {
			defer c.running.Done()
			health := c.check(backend)
			c.mu.Lock()
			defer c.mu.Unlock()
			c.health[name] = health
			c.pending--
		}()
	}
}

// Wait blocks until the running check ends.
func (c *HealthChecker) Wait() {
	c.running.Wait()
}

// Snapshot is every backend's latest health, in order.
func (c *HealthChecker) Snapshot() []Health {
	c.mu.Lock()
	defer c.mu.Unlock()
	snapshot := make([]Health, 0, len(c.Backends))
	for _, name := range c.Backends {
		health, ok := c.health[name]
		switch {
		case c.Disabled:
			health = Health{Backend: name, Label: Labels[name], State: HealthUnavailable, Reason: "check turned off"}
		case !ok:
			health = Health{Backend: name, Label: Labels[name], State: HealthUnavailable, Reason: "not checked yet"}
		}
		snapshot = append(snapshot, health)
	}
	return snapshot
}

func (c *HealthChecker) finished(name string, at time.Time, reason string) Health {
	state := HealthAvailable
	if reason != "" {
		state = HealthUnavailable
	}
	return Health{Backend: name, Label: Labels[name], State: state, Reason: trimReason(reason), CheckedAt: &at}
}

func (c *HealthChecker) check(backend Backend) Health {
	name := backend.Name()
	dir, err := os.MkdirTemp("", "mega-agents-health-")
	if err != nil {
		return c.finished(name, time.Now(), err.Error())
	}
	defer func() { _ = os.RemoveAll(dir) }()
	ctx, cancel := context.WithTimeout(context.Background(), c.Timeout)
	defer cancel()
	started := time.Now()
	reply, err := Runner{}.Run(ctx, backend, Turn{Prompt: HealthCheckPrompt, Dir: dir, Model: HealthCheckModels[name]}, io.Discard)
	elapsed := time.Since(started)
	switch {
	case errors.Is(err, context.DeadlineExceeded):
		return c.finished(name, time.Now(), "timed out after "+formatTimeout(c.Timeout))
	case err != nil:
		return c.finished(name, time.Now(), err.Error())
	case strings.TrimSpace(reply.Text) == "":
		return c.finished(name, time.Now(), "empty reply")
	}
	health := c.finished(name, time.Now(), "")
	health.ReplyMs = elapsed.Milliseconds()
	health.Acknowledged = strings.EqualFold(strings.Trim(reply.Text, " \t\r\n.`'\""), "ACK")
	return health
}

func formatTimeout(timeout time.Duration) string {
	if timeout%time.Second == 0 {
		return fmt.Sprintf("%ds", int(timeout/time.Second))
	}
	return timeout.String()
}

// trimReason keeps a failure's first line, short, so no output dump or
// prompt echo reaches the page.
func trimReason(reason string) string {
	reason, _, _ = strings.Cut(strings.TrimSpace(reason), "\n")
	if runes := []rune(reason); len(runes) > reasonLimit {
		return string(runes[:reasonLimit-1]) + "…"
	}
	return reason
}
