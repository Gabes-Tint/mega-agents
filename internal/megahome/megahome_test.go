package megahome

import (
	"path/filepath"
	"testing"
)

func TestDirPrefersTheEnvironment(t *testing.T) {
	t.Setenv("MEGA_AGENTS_HOME", "/srv/mega")

	if dir, err := Dir(); err != nil || dir != "/srv/mega" {
		t.Fatalf("dir = %q, %v", dir, err)
	}
}

func TestDirDefaultsUnderTheUserHome(t *testing.T) {
	t.Setenv("MEGA_AGENTS_HOME", "")
	t.Setenv("HOME", "/home/someone")

	if dir, err := Dir(); err != nil || dir != filepath.Join("/home/someone", ".mega-agents") {
		t.Fatalf("dir = %q, %v", dir, err)
	}
}

func TestDirExplainsAMissingHome(t *testing.T) {
	t.Setenv("MEGA_AGENTS_HOME", "")
	t.Setenv("HOME", "")

	if _, err := Dir(); err == nil {
		t.Fatal("want an error when no home can be resolved")
	}
}
