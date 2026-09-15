package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	"github.com/Gabes-Tint/mega-agents/internal/agenteval"
)

func main() {
	tool := flag.String("tool", "opencode", "agent tool adapter")
	model := flag.String("model", "", "explicit provider/model passed to the agent tool")
	variant := flag.String("variant", "", "optional provider-specific model reasoning variant")
	executable := flag.String("executable", "opencode", "OpenCode executable path")
	repository := flag.String("repo", ".", "source Git repository")
	output := flag.String("output", "reports/agent-eval/result.json", "JSON result path")
	flag.Parse()

	result, err := agenteval.NewRunner().Run(context.Background(), agenteval.Config{
		RepoRoot: *repository, Tool: *tool, Model: *model, Variant: *variant,
		Executable: *executable, OutputPath: *output,
	}, os.Stdout)
	if err != nil {
		fmt.Fprintln(os.Stderr, "agent evaluation:", err)
		os.Exit(2)
	}
	if !result.Passed {
		os.Exit(1)
	}
}
