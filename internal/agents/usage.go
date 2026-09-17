package agents

import "fmt"

// Usage is what a turn consumed, as its CLI reports it. Input tokens
// include cached ones. CostKnown is false when the CLI reports no price,
// as Codex does on a ChatGPT plan.
type Usage struct {
	InputTokens       int     `json:"inputTokens"`
	CachedInputTokens int     `json:"cachedInputTokens"`
	OutputTokens      int     `json:"outputTokens"`
	CostUSD           float64 `json:"costUsd"`
	CostKnown         bool    `json:"costKnown"`
}

func (usage Usage) Add(other Usage) Usage {
	return Usage{
		InputTokens:       usage.InputTokens + other.InputTokens,
		CachedInputTokens: usage.CachedInputTokens + other.CachedInputTokens,
		OutputTokens:      usage.OutputTokens + other.OutputTokens,
		CostUSD:           usage.CostUSD + other.CostUSD,
		CostKnown:         usage.CostKnown || other.CostKnown,
	}
}

// BudgetError is a conversation that spent its cost budget before its
// reply satisfied the schema.
type BudgetError struct {
	Spent  float64
	Budget float64
	Reason error
}

func (err *BudgetError) Error() string {
	return fmt.Sprintf("the agent spent $%.4f of its $%.4f budget without a valid reply: %v", err.Spent, err.Budget, err.Reason)
}

func (err *BudgetError) Unwrap() error { return err.Reason }

func number(object map[string]any, key string) float64 {
	value, _ := object[key].(float64)
	return value
}

func claudeUsage(envelope map[string]any) Usage {
	usage := objectField(envelope, "usage")
	cached := int(number(usage, "cache_read_input_tokens"))
	cost, known := envelope["total_cost_usd"].(float64)
	return Usage{
		InputTokens:       int(number(usage, "input_tokens")+number(usage, "cache_creation_input_tokens")) + cached,
		CachedInputTokens: cached,
		OutputTokens:      int(number(usage, "output_tokens")),
		CostUSD:           cost,
		CostKnown:         known,
	}
}

func opencodeUsage(events []map[string]any) Usage {
	var total Usage
	for _, event := range events {
		part := objectField(event, "part")
		if stringField(part, "type") != "step-finish" {
			continue
		}
		tokens := objectField(part, "tokens")
		cache := objectField(tokens, "cache")
		cost, known := part["cost"].(float64)
		total = total.Add(Usage{
			InputTokens:       int(number(tokens, "input") + number(cache, "read") + number(cache, "write")),
			CachedInputTokens: int(number(cache, "read")),
			OutputTokens:      int(number(tokens, "output") + number(tokens, "reasoning")),
			CostUSD:           cost,
			CostKnown:         known,
		})
	}
	return total
}

func codexUsage(events []map[string]any) Usage {
	var total Usage
	for _, event := range events {
		if stringField(event, "type") != "turn.completed" {
			continue
		}
		usage := objectField(event, "usage")
		total = total.Add(Usage{
			InputTokens:       int(number(usage, "input_tokens")),
			CachedInputTokens: int(number(usage, "cached_input_tokens")),
			OutputTokens:      int(number(usage, "output_tokens") + number(usage, "reasoning_output_tokens")),
		})
	}
	return total
}
