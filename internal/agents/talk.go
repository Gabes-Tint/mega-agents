package agents

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/Gabes-Tint/mega-agents/internal/schema"
)

const (
	schemaInstruction = "Reply with JSON conforming to the supplied output schema, and nothing else."
	schemaHeading     = "The output schema is:"
)

// Result is a finished conversation: the last reply, and with a schema the
// validated object.
type Result struct {
	Reply    Reply
	Value    map[string]any
	Attempts int
	// Usage adds up every attempt of the conversation.
	Usage Usage
}

// InvalidReplyError is a conversation whose replies never satisfied the
// schema within its retries.
type InvalidReplyError struct {
	Attempts int
	Errors   []schema.FieldError
}

func (err *InvalidReplyError) Error() string {
	messages := make([]string, len(err.Errors))
	for i, fieldError := range err.Errors {
		messages[i] = fieldError.String()
	}
	return fmt.Sprintf("the reply did not satisfy the schema after %d attempts: %s", err.Attempts, strings.Join(messages, "; "))
}

type turnFunc func(context.Context, Turn) (Reply, error)

// Talk sends the turn and, when it carries a schema, holds the reply to it:
// a reply that is not a valid object is answered on the same session with
// what was wrong, up to retries more times. A failed turn ends the
// conversation at once. The context bounds the whole conversation.
func Talk(ctx context.Context, runner Runner, backend Backend, turn Turn, retries int, log io.Writer) (Result, error) {
	run := func(ctx context.Context, turn Turn) (Reply, error) {
		return runner.Run(ctx, backend, turn, log)
	}
	return talk(ctx, backend, turn, retries, run, log)
}

func talk(ctx context.Context, backend Backend, turn Turn, retries int, run turnFunc, log io.Writer) (Result, error) {
	if turn.Schema == nil {
		reply, err := run(ctx, turn)
		return Result{Reply: reply, Attempts: 1, Usage: reply.Usage}, err
	}
	instruction := schemaInstruction
	if !backend.EnforcesSchema() {
		// The model was never handed the schema, so the instruction carries
		// it, on every attempt: models lose it between turns.
		instruction += "\n\n" + schemaHeading + "\n" + turn.Schema.Text
	}
	attempt := turn
	attempt.Prompt = turn.Prompt + "\n\n" + instruction
	var usage Usage
	for number := 1; ; number++ {
		fmt.Fprintf(log, "🔁 Attempt %d\n", number)
		reply, err := run(ctx, attempt)
		usage = usage.Add(reply.Usage)
		if err != nil {
			return Result{Reply: reply, Attempts: number, Usage: usage}, err
		}
		value, errs := validate(turn.Schema, reply)
		if len(errs) == 0 {
			return Result{Reply: reply, Value: value, Attempts: number, Usage: usage}, nil
		}
		correction := errs[0].String()
		fmt.Fprintf(log, "⚠️ attempt %d did not satisfy the schema: %s\n", number, correction)
		invalid := &InvalidReplyError{Attempts: number, Errors: errs}
		if number > retries {
			return Result{Reply: reply, Attempts: number, Usage: usage}, invalid
		}
		if turn.MaxCostUSD > 0 && usage.CostKnown && usage.CostUSD > turn.MaxCostUSD {
			fmt.Fprintf(log, "💸 the conversation has cost $%.4f, over its $%.4f budget; not retrying\n", usage.CostUSD, turn.MaxCostUSD)
			return Result{Reply: reply, Attempts: number, Usage: usage}, &BudgetError{Spent: usage.CostUSD, Budget: turn.MaxCostUSD, Reason: invalid}
		}
		if reply.SessionID != "" {
			// Repairs continue the conversation the first attempt started,
			// which is already this turn's own copy when it forked.
			attempt.SessionID, attempt.Fork = reply.SessionID, false
		}
		// The rejected reply is not quoted back: the session holds it, and
		// the model's own text is data rather than instruction.
		attempt.Prompt = "That reply was rejected: " + correction + "\n\n" + instruction
	}
}

func validate(compiled *schema.Schema, reply Reply) (map[string]any, []schema.FieldError) {
	if reply.Structured == nil {
		return nil, []schema.FieldError{{Path: "$", Message: "the reply was not a JSON object"}}
	}
	return reply.Structured, compiled.Validate(normalize(reply.Structured))
}

// normalize re-encodes a parsed object the way the schema validator expects
// numbers to look.
func normalize(object map[string]any) any {
	value, _ := schema.Normalize(object)
	return value
}
