// Package megahome locates the folder where Mega Agents keeps its own state:
// run records, logs and worktrees.
package megahome

import (
	"fmt"
	"os"
	"path/filepath"
)

// Dir is $MEGA_AGENTS_HOME, or ~/.mega-agents when that is unset.
func Dir() (string, error) {
	if home := os.Getenv("MEGA_AGENTS_HOME"); home != "" {
		return home, nil
	}
	userHome, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("cannot resolve the Mega Agents home; set MEGA_AGENTS_HOME: %w", err)
	}
	return filepath.Join(userHome, ".mega-agents"), nil
}
