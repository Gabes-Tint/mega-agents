package app

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Gabes-Tint/mega-agents/internal/agents"
)

func agentHealth(t *testing.T, handler http.Handler, method string) (int, AgentHealthResponse) {
	t.Helper()
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(method, "/api/agents/health", nil))
	var body AgentHealthResponse
	if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	return response.Code, body
}

// onlyFakeClaude leaves PATH holding just a claude that answers ACK, so no
// real agent CLI is ever reached.
func onlyFakeClaude(t *testing.T) string {
	t.Helper()
	bin := t.TempDir()
	script := "#!/bin/sh\necho run >> \"${0%/*}/runs\"\necho '{\"type\":\"result\",\"session_id\":\"s1\",\"result\":\"ACK\"}'\n"
	if err := os.WriteFile(filepath.Join(bin, "claude"), []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin)
	return bin
}

func TestAgentHealthEndpointReportsEachBackend(t *testing.T) {
	onlyFakeClaude(t)
	health := agents.NewHealthChecker()
	health.Start()
	health.Wait()

	code, body := agentHealth(t, NewHandlerWithHealth(testAssets(), health), http.MethodGet)

	if code != http.StatusOK || len(body.Agents) != 4 {
		t.Fatalf("GET = %d %+v", code, body)
	}
	claude, codex := body.Agents[0], body.Agents[1]
	if claude.Backend != "claude" || claude.Label != "Claude Code" || claude.State != agents.HealthAvailable || claude.CheckedAt == nil {
		t.Errorf("claude = %+v, want available", claude)
	}
	if codex.State != agents.HealthUnavailable || codex.Reason != "not installed" {
		t.Errorf("codex = %+v, want unavailable: not installed", codex)
	}
}

func TestAgentHealthEndpointRerunsTheCheckOnPost(t *testing.T) {
	bin := onlyFakeClaude(t)
	health := agents.NewHealthChecker()
	handler := NewHandlerWithHealth(testAssets(), health)

	code, body := agentHealth(t, handler, http.MethodPost)
	health.Wait()

	if code != http.StatusAccepted || len(body.Agents) != 4 {
		t.Fatalf("POST = %d %+v", code, body)
	}
	if runs, _ := os.ReadFile(filepath.Join(bin, "runs")); strings.Count(string(runs), "run") != 1 {
		t.Fatalf("runs = %q, want the check to run once", runs)
	}
	if _, body := agentHealth(t, handler, http.MethodGet); body.Agents[0].State != agents.HealthAvailable {
		t.Fatalf("claude after the check = %+v", body.Agents[0])
	}
}

func TestAgentHealthCheckCanBeTurnedOffFromTheEnvironment(t *testing.T) {
	bin := onlyFakeClaude(t)
	t.Setenv("MEGA_AGENTS_SKIP_AGENT_CHECK", "1")

	health := AgentHealthFromEnvironment()
	health.Start()
	health.Wait()

	if _, err := os.Stat(filepath.Join(bin, "runs")); !os.IsNotExist(err) {
		t.Fatal("a turned-off check ran the CLI")
	}
	if _, body := agentHealth(t, NewHandlerWithHealth(testAssets(), health), http.MethodPost); body.Agents[0].Reason != "check turned off" {
		t.Fatalf("claude = %+v, want check turned off", body.Agents[0])
	}
}

func TestAgentHealthCheckIsOnByDefault(t *testing.T) {
	for _, value := range []string{"", "0", "false"} {
		t.Setenv("MEGA_AGENTS_SKIP_AGENT_CHECK", value)
		if AgentHealthFromEnvironment().Disabled {
			t.Errorf("MEGA_AGENTS_SKIP_AGENT_CHECK=%q turned the check off", value)
		}
	}
}

func TestPlainHandlerNeverChecksAgents(t *testing.T) {
	bin := onlyFakeClaude(t)

	code, body := agentHealth(t, NewHandler(testAssets()), http.MethodGet)

	if code != http.StatusOK || body.Agents[0].Reason != "not checked yet" {
		t.Fatalf("GET = %d %+v", code, body)
	}
	if _, err := os.Stat(filepath.Join(bin, "runs")); !os.IsNotExist(err) {
		t.Fatal("building a handler ran an agent CLI")
	}
}
