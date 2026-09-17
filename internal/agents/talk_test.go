package agents

import (
	"context"
	"errors"
	"strings"
	"testing"
)

// scripted answers each attempt with the next reply and records the turns.
type scripted struct {
	replies []Reply
	errs    []error
	turns   []Turn
}

func (s *scripted) run(_ context.Context, turn Turn) (Reply, error) {
	s.turns = append(s.turns, turn)
	i := len(s.turns) - 1
	if i < len(s.errs) && s.errs[i] != nil {
		return Reply{}, s.errs[i]
	}
	return s.replies[i], nil
}

func TestTalkWithoutASchemaIsOneTurn(t *testing.T) {
	agent := &scripted{replies: []Reply{{SessionID: "s1", Text: "done"}}}

	result, err := talk(context.Background(), claude{}, Turn{Prompt: "fix it"}, 2, agent.run, &strings.Builder{})

	if err != nil || result.Reply.Text != "done" || len(agent.turns) != 1 || agent.turns[0].Prompt != "fix it" || result.Attempts != 1 {
		t.Fatalf("result = %+v, turns = %+v, err = %v", result, agent.turns, err)
	}
}

func TestTalkRepairsAReplyOnTheSameSession(t *testing.T) {
	agent := &scripted{replies: []Reply{
		{SessionID: "s1", Text: "I think it is fine"},
		{SessionID: "s1", Text: `{"verdict": 3}`, Structured: map[string]any{"verdict": 3.0}},
		{SessionID: "s1", Text: `{"verdict": "ok"}`, Structured: map[string]any{"verdict": "ok"}},
	}}
	var log strings.Builder

	result, err := talk(context.Background(), claude{}, Turn{Prompt: "review", Schema: strictSchema(t)}, 2, agent.run, &log)

	if err != nil || result.Value["verdict"] != "ok" || result.Attempts != 3 {
		t.Fatalf("result = %+v, err = %v", result, err)
	}
	instruction := "Reply with JSON conforming to the supplied output schema, and nothing else."
	if agent.turns[0].Prompt != "review\n\n"+instruction || agent.turns[0].SessionID != "" {
		t.Fatalf("first turn = %+v", agent.turns[0])
	}
	if agent.turns[1].SessionID != "s1" || agent.turns[1].Prompt != "That reply was rejected: $: the reply was not a JSON object\n\n"+instruction {
		t.Fatalf("repair turn = %+v", agent.turns[1])
	}
	if !strings.Contains(agent.turns[2].Prompt, "That reply was rejected: $.verdict: ") {
		t.Fatalf("second repair = %q", agent.turns[2].Prompt)
	}
	if !strings.Contains(log.String(), "attempt 1 did not satisfy the schema: $: the reply was not a JSON object") {
		t.Fatalf("log = %s", log.String())
	}
}

func TestTalkCarriesTheSchemaInThePromptForBackendsWithoutAFlag(t *testing.T) {
	compiled := strictSchema(t)
	agent := &scripted{replies: []Reply{
		{SessionID: "o1", Text: "nope"},
		{SessionID: "o1", Text: `{"verdict":"ok"}`, Structured: map[string]any{"verdict": "ok"}},
	}}

	if _, err := talk(context.Background(), opencode{}, Turn{Prompt: "review", Schema: compiled}, 1, agent.run, &strings.Builder{}); err != nil {
		t.Fatal(err)
	}
	for i, turn := range agent.turns {
		if !strings.HasSuffix(turn.Prompt, "The output schema is:\n"+compiled.Text) {
			t.Errorf("turn %d prompt = %q, want the schema at its end", i+1, turn.Prompt)
		}
	}
}

func TestTalkGivesUpAfterItsRetries(t *testing.T) {
	agent := &scripted{replies: []Reply{{SessionID: "s1", Text: "no"}, {SessionID: "s1", Text: "still no"}}}

	result, err := talk(context.Background(), claude{}, Turn{Prompt: "review", Schema: strictSchema(t)}, 1, agent.run, &strings.Builder{})

	var invalid *InvalidReplyError
	if !errors.As(err, &invalid) || invalid.Attempts != 2 || len(invalid.Errors) != 1 || result.Reply.Text != "still no" {
		t.Fatalf("result = %+v, err = %v", result, err)
	}
	if !strings.Contains(err.Error(), "the reply did not satisfy the schema after 2 attempts: $: the reply was not a JSON object") {
		t.Fatalf("err = %v", err)
	}
}

func TestTalkStopsAtAFailedTurn(t *testing.T) {
	agent := &scripted{errs: []error{errors.New("claude reported an error: overloaded")}}

	_, err := talk(context.Background(), claude{}, Turn{Prompt: "review", Schema: strictSchema(t)}, 2, agent.run, &strings.Builder{})

	if err == nil || err.Error() != "claude reported an error: overloaded" || len(agent.turns) != 1 {
		t.Fatalf("err = %v, turns = %d", err, len(agent.turns))
	}
}

func TestTalkRunsThroughTheRunner(t *testing.T) {
	fakeCLI(t, "claude", `echo '{"type":"result","session_id":"s9","result":"{\"verdict\":\"ok\"}","structured_output":{"verdict":"ok"}}'`)

	result, err := Talk(context.Background(), Runner{}, claude{}, Turn{Prompt: "p", Dir: t.TempDir(), Schema: strictSchema(t)}, 0, &strings.Builder{})

	if err != nil || result.Reply.SessionID != "s9" || result.Value["verdict"] != "ok" {
		t.Fatalf("result = %+v, err = %v", result, err)
	}
}

func TestRepairsOfAForkedTurnContinueTheFork(t *testing.T) {
	agent := &scripted{replies: []Reply{
		{SessionID: "copy", Text: "no"},
		{SessionID: "copy", Text: `{"verdict":"ok"}`, Structured: map[string]any{"verdict": "ok"}},
	}}

	if _, err := talk(context.Background(), claude{}, Turn{Prompt: "p", SessionID: "original", Fork: true, Schema: strictSchema(t)}, 1, agent.run, &strings.Builder{}); err != nil {
		t.Fatal(err)
	}
	first, repair := agent.turns[0], agent.turns[1]
	if first.SessionID != "original" || !first.Fork || repair.SessionID != "copy" || repair.Fork {
		t.Fatalf("turns = %+v", agent.turns)
	}
}
