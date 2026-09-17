package engine

import "testing"

func TestEveryStatusHasItsEmoji(t *testing.T) {
	t.Parallel()

	for status, want := range map[Status]string{
		Pending:       "⏳",
		Running:       "▶️",
		Succeeded:     "✅",
		Failed:        "❌",
		Skipped:       "⏭️",
		"cancelled":   "🛑",
		"interrupted": "⚠️",
		"unheard-of":  "•",
	} {
		if got := status.Emoji(); got != want {
			t.Errorf("%s emoji = %q, want %q", status, got, want)
		}
	}
}
