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
		return Result{Reply: reply, Attempts: 1}, err
	}
	instruction := schemaInstruction
	if !backend.EnforcesSchema() {
		// The model was never handed the schema, so the instruction carries
		// it, on every attempt: models lose it between turns.
		instruction += "\n\n" + schemaHeading + "\n" + turn.Schema.Text
	}
	attempt := turn
	attempt.Prompt = turn.Prompt + "\n\n" + instruction
	for number := 1; ; number++ {
		fmt.Fprintf(log, "Attempt %d\n", number)
		reply, err := run(ctx, attempt)
		if err != nil {
			return Result{Reply: reply, Attempts: number}, err
		}
		value, errs := validate(turn.Schema, reply)
		if len(errs) == 0 {
			return Result{Reply: reply, Value: value, Attempts: number}, nil
		}
		correction := errs[0].String()
		fmt.Fprintf(log, "attempt %d did not satisfy the schema: %s\n", number, correction)
		if number > retries {
			return Result{Reply: reply, Attempts: number}, &InvalidReplyError{Attempts: number, Errors: errs}
		}
		if reply.SessionID != "" {
			attempt.SessionID = reply.SessionID
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
