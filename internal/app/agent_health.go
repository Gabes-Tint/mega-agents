package app

import (
	"encoding/json"
	"net/http"
	"os"

	"github.com/Gabes-Tint/mega-agents/internal/agents"
)

// SkipAgentCheckVariable turns the startup agent check off, so tests and CI
// never spend a model turn.
const SkipAgentCheckVariable = "MEGA_AGENTS_SKIP_AGENT_CHECK"

type AgentHealthResponse struct {
	Agents []agents.Health `json:"agents"`
}

// AgentHealthFromEnvironment is the checker the server starts with, turned
// off when SkipAgentCheckVariable is set to anything but 0 or false.
func AgentHealthFromEnvironment() *agents.HealthChecker {
	health := agents.NewHealthChecker()
	switch os.Getenv(SkipAgentCheckVariable) {
	case "", "0", "false":
	default:
		health.Disabled = true
	}
	return health
}

func registerAgentHealthHandler(mux *http.ServeMux, health *agents.HealthChecker) {
	write := func(w http.ResponseWriter, code int) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(code)
		_ = json.NewEncoder(w).Encode(AgentHealthResponse{Agents: health.Snapshot()})
	}
	mux.HandleFunc("GET /api/agents/health", func(w http.ResponseWriter, _ *http.Request) {
		write(w, http.StatusOK)
	})
	mux.HandleFunc("POST /api/agents/health", func(w http.ResponseWriter, _ *http.Request) {
		health.Start()
		write(w, http.StatusAccepted)
	})
}
