package engine

// Emoji marks a status in logs and command-line output so success, failure
// and warnings stand out at a glance. It also knows the statuses the run
// store adds for runs that were cancelled or interrupted.
func (status Status) Emoji() string {
	switch status {
	case Pending:
		return "⏳"
	case Running:
		return "▶️"
	case Succeeded:
		return "✅"
	case Failed:
		return "❌"
	case Skipped:
		return "⏭️"
	case Paused:
		return "⏸️"
	case "cancelled":
		return "🛑"
	case "interrupted":
		return "⚠️"
	}
	return "•"
}
