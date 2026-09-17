package agents

import (
	"context"
	"errors"
	"strings"
	"testing"
)

// Lines captured from real turns on 2026-09-17.
const (
	claudeResult = `{"type":"result","is_error":false,"session_id":"9fb8","result":"ok","total_cost_usd":0.017527700000000004,"usage":{"input_tokens":10,"cache_creation_input_tokens":7505,"cache_read_input_tokens":13607,"output_tokens":39}}`
	opencodeRun  = `{"type":"step_start","sessionID":"ses_1","part":{"id":"prt_a","type":"step-start"}}
{"type":"text","sessionID":"ses_1","part":{"id":"prt_b","type":"text","text":"ok"}}
{"type":"step_finish","sessionID":"ses_1","part":{"id":"prt_c","type":"step-finish","tokens":{"total":7822,"input":5515,"output":3,"reasoning":0,"cache":{"write":0,"read":2304}},"cost":0.00089787}}
{"type":"step_finish","sessionID":"ses_1","part":{"id":"prt_d","type":"step-finish","tokens":{"total":100,"input":90,"output":6,"reasoning":4,"cache":{"write":0,"read":0}},"cost":0.0001}}`
	codexRun = `{"type":"thread.started","thread_id":"t1"}
{"type":"item.completed","item":{"id":"item_0","type":"agent_message","text":"ok"}}
{"type":"turn.completed","usage":{"input_tokens":15681,"cached_input_tokens":6912,"cache_write_input_tokens":0,"output_tokens":5,"reasoning_output_tokens":2}}`
)

func TestRepliesReportTheirUsage(t *testing.T) {
	cases := map[string]struct {
		stdout string
		want   Usage
	}{
		"claude":   {claudeResult, Usage{InputTokens: 21122, CachedInputTokens: 13607, OutputTokens: 39, CostUSD: 0.017527700000000004, CostKnown: true}},
		"opencode": {opencodeRun, Usage{InputTokens: 7909, CachedInputTokens: 2304, OutputTokens: 13, CostUSD: 0.00099787, CostKnown: true}},
		"codex":    {codexRun, Usage{InputTokens: 15681, CachedInputTokens: 6912, OutputTokens: 7}},
		"grok":     {`{"sessionId":"g1","text":"ok"}`, Usage{}},
	}
	for name, testCase := range cases {
		t.Run(name, func(t *testing.T) {
			backend, _ := Lookup(name)
			reply, err := backend.Parse(Output{Stdout: testCase.stdout}, Turn{Prompt: "p"})
			if err != nil {
				t.Fatal(err)
			}
			got := reply.Usage
			got.CostUSD = round(got.CostUSD)
			want := testCase.want
			want.CostUSD = round(want.CostUSD)
			if got != want {
				t.Fatalf("usage = %+v, want %+v", got, want)
			}
		})
	}
}

func round(value float64) float64 {
	return float64(int64(value*1e9+0.5)) / 1e9
}

func TestUsageAddsUp(t *testing.T) {
	total := Usage{InputTokens: 10, OutputTokens: 1, CostUSD: 0.5, CostKnown: true}
	total = total.Add(Usage{InputTokens: 5, CachedInputTokens: 2, OutputTokens: 3})
	total = total.Add(Usage{CostUSD: 0.25, CostKnown: true})

	if total != (Usage{InputTokens: 15, CachedInputTokens: 2, OutputTokens: 4, CostUSD: 0.75, CostKnown: true}) {
		t.Fatalf("total = %+v", total)
	}
}

func TestAConversationAddsTheUsageOfEveryAttempt(t *testing.T) {
	agent := &scripted{replies: []Reply{
		{SessionID: "s1", Text: "no", Usage: Usage{InputTokens: 100, OutputTokens: 10, CostUSD: 0.01, CostKnown: true}},
		{SessionID: "s1", Text: `{"verdict":"ok"}`, Structured: map[string]any{"verdict": "ok"}, Usage: Usage{InputTokens: 50, OutputTokens: 5, CostUSD: 0.02, CostKnown: true}},
	}}

	result, err := talk(context.Background(), claude{}, Turn{Prompt: "p", Schema: strictSchema(t)}, 2, agent.run, &strings.Builder{})

	if err != nil || result.Usage != (Usage{InputTokens: 150, OutputTokens: 15, CostUSD: 0.03, CostKnown: true}) {
		t.Fatalf("result = %+v, %v", result.Usage, err)
	}
}

func TestACostBudgetStopsRepairTurns(t *testing.T) {
	agent := &scripted{replies: []Reply{
		{SessionID: "s1", Text: "no", Usage: Usage{CostUSD: 0.06, CostKnown: true}},
		{SessionID: "s1", Text: "still no", Usage: Usage{CostUSD: 0.06, CostKnown: true}},
		{SessionID: "s1", Text: `{"verdict":"ok"}`, Structured: map[string]any{"verdict": "ok"}},
	}}
	turn := Turn{Prompt: "p", Schema: strictSchema(t), MaxCostUSD: 0.1}

	result, err := talk(context.Background(), claude{}, turn, 5, agent.run, &strings.Builder{})

	var over *BudgetError
	if !errors.As(err, &over) || len(agent.turns) != 2 || round(result.Usage.CostUSD) != 0.12 {
		t.Fatalf("err = %v, turns = %d, usage = %+v", err, len(agent.turns), result.Usage)
	}
	if !strings.Contains(err.Error(), "spent $0.1200 of its $0.1000 budget") {
		t.Fatalf("err = %v", err)
	}
}
