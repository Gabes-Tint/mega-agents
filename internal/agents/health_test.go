package agents

import (
	"os"
	"strings"
	"testing"
	"time"
)

func healthOf(t *testing.T, checker *HealthChecker, backend string) Health {
	t.Helper()
	for _, health := range checker.Snapshot() {
		if health.Backend == backend {
			return health
		}
	}
	t.Fatalf("no health reported for %s in %+v", backend, checker.Snapshot())
	return Health{}
}

func TestHealthCheckerListsEveryBackendWithItsLabelBeforeAnyCheck(t *testing.T) {
	checker := NewHealthChecker()

	snapshot := checker.Snapshot()

	want := []struct{ backend, label string }{
		{"claude", "Claude Code"}, {"codex", "Codex"}, {"grok", "Grok"}, {"opencode", "OpenCode"},
	}
	if len(snapshot) != len(want) {
		t.Fatalf("snapshot = %+v", snapshot)
	}
	for i, health := range snapshot {
		if health.Backend != want[i].backend || health.Label != want[i].label || health.State != HealthUnavailable || health.Reason != "not checked yet" {
			t.Errorf("health %d = %+v, want %s (%s) unavailable, not checked yet", i, health, want[i].backend, want[i].label)
		}
	}
}

func TestHealthCheckerMarksABackendAvailableWhenItRepliesOnItsCheapestModel(t *testing.T) {
	bin := fakeCLI(t, "claude", `
printf '%s\n' "$@" > "$(dirname "$0")/args"
pwd > "$(dirname "$0")/cwd"
echo '{"type":"result","session_id":"s1","result":"ACK"}'
`)
	checker := NewHealthChecker()
	checker.Backends = []string{"claude"}
	before := time.Now()

	checker.Start()
	if state := healthOf(t, checker, "claude").State; state != HealthChecking {
		t.Fatalf("state while checking = %s, want checking", state)
	}
	checker.Wait()

	health := healthOf(t, checker, "claude")
	if health.State != HealthAvailable || health.Reason != "" || !health.Acknowledged {
		t.Fatalf("health = %+v, want available and acknowledged", health)
	}
	if health.CheckedAt == nil || health.CheckedAt.Before(before) {
		t.Fatalf("checkedAt = %v, want a time after %v", health.CheckedAt, before)
	}
	args, _ := os.ReadFile(bin + "/args")
	if !strings.Contains(string(args), "--model\nhaiku\n") || !strings.Contains(string(args), "Reply with exactly: ACK") {
		t.Fatalf("args = %q, want the ACK prompt on haiku", args)
	}
	cwd, _ := os.ReadFile(bin + "/cwd")
	if dir := strings.TrimSpace(string(cwd)); !strings.Contains(dir, "mega-agents-health-") {
		t.Fatalf("cwd = %q, want a temporary directory", dir)
	}
	if _, err := os.Stat(strings.TrimSpace(string(cwd))); !os.IsNotExist(err) {
		t.Fatalf("the temporary directory was left behind: %v", err)
	}
}

func TestHealthCheckerCountsAnyNonEmptyReplyAndRecordsItWasNotAnACK(t *testing.T) {
	fakeCLI(t, "claude", `echo '{"type":"result","session_id":"s1","result":"Sure! ACK."}'`)
	checker := NewHealthChecker()
	checker.Backends = []string{"claude"}

	checker.Start()
	checker.Wait()

	if health := healthOf(t, checker, "claude"); health.State != HealthAvailable || health.Acknowledged {
		t.Fatalf("health = %+v, want available but not acknowledged", health)
	}
}

func TestHealthCheckerMarksAnEmptyReplyUnavailable(t *testing.T) {
	fakeCLI(t, "claude", `echo '{"type":"result","session_id":"s1","result":"  "}'`)
	checker := NewHealthChecker()
	checker.Backends = []string{"claude"}

	checker.Start()
	checker.Wait()

	if health := healthOf(t, checker, "claude"); health.State != HealthUnavailable || health.Reason != "empty reply" {
		t.Fatalf("health = %+v, want unavailable with an empty reply", health)
	}
}

func TestHealthCheckerKeepsOnlyTheFirstLineOfAFailure(t *testing.T) {
	fakeCLI(t, "claude", `echo 'secret stderr dump' >&2; exit 3`)
	checker := NewHealthChecker()
	checker.Backends = []string{"claude"}

	checker.Start()
	checker.Wait()

	health := healthOf(t, checker, "claude")
	if health.State != HealthUnavailable || health.Reason != "claude exited 3" {
		t.Fatalf("health = %+v, want unavailable: claude exited 3", health)
	}
}

func TestHealthCheckerTrimsALongReason(t *testing.T) {
	fakeCLI(t, "claude", `echo '{"type":"result","is_error":true,"result":"`+strings.Repeat("x", 500)+`"}'`)
	checker := NewHealthChecker()
	checker.Backends = []string{"claude"}

	checker.Start()
	checker.Wait()

	if reason := healthOf(t, checker, "claude").Reason; len([]rune(reason)) > 120 || !strings.HasSuffix(reason, "…") {
		t.Fatalf("reason = %q (%d runes), want at most 120 ending in an ellipsis", reason, len([]rune(reason)))
	}
}

func TestHealthCheckerMarksAMissingCLINotInstalledWithoutRunningAnything(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	checker := NewHealthChecker()

	checker.Start()
	for _, health := range checker.Snapshot() {
		if health.State != HealthUnavailable || health.Reason != "not installed" {
			t.Errorf("health right after start = %+v, want unavailable: not installed", health)
		}
	}
	checker.Wait()
}

func TestHealthCheckerGivesUpAfterItsTimeout(t *testing.T) {
	fakeCLI(t, "claude", `sleep 10`)
	checker := NewHealthChecker()
	checker.Backends = []string{"claude"}
	checker.Timeout = 100 * time.Millisecond
	started := time.Now()

	checker.Start()
	checker.Wait()

	if elapsed := time.Since(started); elapsed > 5*time.Second {
		t.Fatalf("the check took %s, want it stopped at its timeout", elapsed)
	}
	if health := healthOf(t, checker, "claude"); health.State != HealthUnavailable || health.Reason != "timed out after 100ms" {
		t.Fatalf("health = %+v, want unavailable: timed out after 100ms", health)
	}
}

func TestHealthCheckerDoesNotStartASecondCheckWhileOneRuns(t *testing.T) {
	bin := fakeCLI(t, "claude", `echo run >> "$(dirname "$0")/runs"; sleep 0.3; echo '{"type":"result","session_id":"s1","result":"ACK"}'`)
	checker := NewHealthChecker()
	checker.Backends = []string{"claude"}

	checker.Start()
	checker.Start()
	checker.Wait()

	if runs, _ := os.ReadFile(bin + "/runs"); strings.Count(string(runs), "run") != 1 {
		t.Fatalf("runs = %q, want one", runs)
	}
	checker.Start()
	checker.Wait()
	if runs, _ := os.ReadFile(bin + "/runs"); strings.Count(string(runs), "run") != 2 {
		t.Fatalf("runs = %q, want a second check once the first ended", runs)
	}
}

func TestHealthCheckerTurnedOffRunsNothing(t *testing.T) {
	bin := fakeCLI(t, "claude", `touch "$(dirname "$0")/ran"`)
	checker := NewHealthChecker()
	checker.Backends = []string{"claude"}
	checker.Disabled = true

	checker.Start()
	checker.Wait()

	if _, err := os.Stat(bin + "/ran"); !os.IsNotExist(err) {
		t.Fatal("a turned-off check ran the CLI")
	}
	for _, health := range checker.Snapshot() {
		if health.State != HealthUnavailable || health.Reason != "check turned off" {
			t.Errorf("health = %+v, want unavailable: check turned off", health)
		}
	}
}

func TestHealthCheckerUsesEachBackendsCheapestModel(t *testing.T) {
	want := map[string]string{"claude": "haiku", "opencode": "opencode-go/glm-5.3-flash", "codex": "", "grok": ""}
	for backend, model := range want {
		if got := HealthCheckModels[backend]; got != model {
			t.Errorf("%s check model = %q, want %q", backend, got, model)
		}
	}
}
