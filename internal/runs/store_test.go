package runs

import (
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/Gabes-Tint/mega-agents/internal/engine"
)

func newStore(t *testing.T) *Store {
	t.Helper()
	return &Store{Root: t.TempDir(), Now: func() time.Time {
		return time.Date(2026, 9, 16, 23, 30, 0, 0, time.UTC)
	}}
}

func TestCreateRecordsARunningRunOnDisk(t *testing.T) {
	store := newStore(t)
	steps := []engine.Step{{TaskID: "a1", Name: "Fetch", Kind: "fetch", Status: engine.Pending}}

	record, err := store.Create("issue-to-pr", steps)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if !regexp.MustCompile(`^20260916T233000Z-[0-9a-f]{6}$`).MatchString(record.ID) {
		t.Fatalf("id = %q, want a time-sortable id", record.ID)
	}
	loaded, err := store.Load(record.ID)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if loaded.Workflow != "issue-to-pr" || loaded.Status != engine.Running || loaded.StartedAt != "2026-09-16T23:30:00Z" ||
		loaded.PID != os.Getpid() || len(loaded.Steps) != 1 || loaded.Steps[0].Name != "Fetch" {
		t.Fatalf("loaded = %+v", loaded)
	}
}

func TestSaveReplacesTheRecord(t *testing.T) {
	store := newStore(t)
	record, _ := store.Create("flow", nil)
	record.Status = engine.Succeeded
	record.FinishedAt = "2026-09-16T23:31:00Z"

	if err := store.Save(record); err != nil {
		t.Fatalf("save: %v", err)
	}
	loaded, _ := store.Load(record.ID)
	if loaded.Status != engine.Succeeded || loaded.FinishedAt != "2026-09-16T23:31:00Z" {
		t.Fatalf("loaded = %+v", loaded)
	}
}

func TestARunningRecordWhoseProcessIsGoneIsInterrupted(t *testing.T) {
	store := newStore(t)
	record, _ := store.Create("flow", []engine.Step{{TaskID: "a1", Status: engine.Running}})
	finished := exec.Command("true")
	if err := finished.Run(); err != nil {
		t.Fatalf("cannot start a short-lived process: %v", err)
	}
	record.PID = finished.Process.Pid
	if err := store.Save(record); err != nil {
		t.Fatal(err)
	}

	loaded, err := store.Load(record.ID)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if loaded.Status != Interrupted || loaded.Steps[0].Status != Interrupted {
		t.Fatalf("loaded = %+v, want the run and its running step interrupted", loaded)
	}
}

func TestListShowsTheNewestRunFirst(t *testing.T) {
	store := newStore(t)
	clock := time.Date(2026, 9, 16, 23, 0, 0, 0, time.UTC)
	store.Now = func() time.Time {
		clock = clock.Add(time.Minute)
		return clock
	}
	first, _ := store.Create("first", nil)
	second, _ := store.Create("second", nil)

	records, err := store.List()
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(records) != 2 || records[0].ID != second.ID || records[1].ID != first.ID {
		t.Fatalf("records = %+v", records)
	}
}

func TestListOfAnEmptyStore(t *testing.T) {
	store := &Store{Root: filepath.Join(t.TempDir(), "never-created")}

	records, err := store.List()
	if err != nil || len(records) != 0 {
		t.Fatalf("records = %+v, %v", records, err)
	}
}

func TestLogsAppendPerStep(t *testing.T) {
	store := newStore(t)
	record, _ := store.Create("flow", nil)

	for _, line := range []string{"$ git fetch origin\n", "Fetch succeeded\n"} {
		log, err := store.OpenLog(record.ID, "a1")
		if err != nil {
			t.Fatalf("open log: %v", err)
		}
		if _, err := io.WriteString(log, line); err != nil {
			t.Fatal(err)
		}
		if err := log.Close(); err != nil {
			t.Fatal(err)
		}
	}

	text, err := store.ReadLog(record.ID, "a1")
	if err != nil || text != "$ git fetch origin\nFetch succeeded\n" {
		t.Fatalf("log = %q, %v", text, err)
	}
}

func TestMissingRunsAndLogsAreNotFound(t *testing.T) {
	store := newStore(t)
	record, _ := store.Create("flow", nil)

	if _, err := store.Load("20260916T233000Z-000000"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("load err = %v", err)
	}
	if _, err := store.ReadLog(record.ID, "never-ran"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("read log err = %v", err)
	}
}

func TestIdentifiersCannotEscapeTheStore(t *testing.T) {
	store := newStore(t)
	record, _ := store.Create("flow", nil)

	for _, id := range []string{"../escape", "a/b", "", ".."} {
		if _, err := store.Load(id); !errors.Is(err, ErrNotFound) {
			t.Errorf("Load(%q) err = %v, want not found", id, err)
		}
		if _, err := store.OpenLog(record.ID, id); err == nil || !strings.Contains(err.Error(), "invalid") {
			t.Errorf("OpenLog(%q) err = %v, want invalid", id, err)
		}
		if _, err := store.ReadLog(record.ID, id); !errors.Is(err, ErrNotFound) {
			t.Errorf("ReadLog(%q) err = %v, want not found", id, err)
		}
	}
}

func TestDefaultRootIsUnderTheMegaAgentsHome(t *testing.T) {
	home := t.TempDir()
	t.Setenv("MEGA_AGENTS_HOME", home)

	store, err := DefaultStore()
	if err != nil || store.Root != filepath.Join(home, "runs") {
		t.Fatalf("store = %+v, %v", store, err)
	}
}

func TestAnUnreadableRecordIsAnError(t *testing.T) {
	store := newStore(t)
	record, _ := store.Create("flow", nil)
	if err := os.WriteFile(filepath.Join(store.Root, record.ID, "run.json"), []byte("{"), 0o600); err != nil {
		t.Fatal(err)
	}

	if _, err := store.Load(record.ID); err == nil || !strings.Contains(err.Error(), "unreadable record") {
		t.Fatalf("load err = %v", err)
	}
	if _, err := store.List(); err == nil {
		t.Fatal("list must surface an unreadable record")
	}
}

func TestSaveRejectsAnInvalidID(t *testing.T) {
	store := newStore(t)

	if err := store.Save(Record{ID: "../x"}); err == nil || !strings.Contains(err.Error(), "invalid run id") {
		t.Fatalf("save err = %v", err)
	}
}

func TestListSkipsFoldersWithoutARecord(t *testing.T) {
	store := newStore(t)
	record, _ := store.Create("flow", nil)
	if err := os.MkdirAll(filepath.Join(store.Root, "stray"), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(store.Root, "note.txt"), nil, 0o600); err != nil {
		t.Fatal(err)
	}

	records, err := store.List()
	if err != nil || len(records) != 1 || records[0].ID != record.ID {
		t.Fatalf("records = %+v, %v", records, err)
	}
}

func TestCreateFailsWhenTheStoreCannotBeWritten(t *testing.T) {
	root := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(root, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	store := &Store{Root: root}

	if _, err := store.Create("flow", nil); err == nil || !strings.Contains(err.Error(), "cannot create the run folder") {
		t.Fatalf("create err = %v", err)
	}
	if _, err := store.OpenLog("20260916T233000Z-000000", "a1"); err == nil {
		t.Fatal("open log must fail without a run folder")
	}
}

func TestADeadProcessIDIsNotAlive(t *testing.T) {
	if processAlive(0) || processAlive(-1) {
		t.Fatal("non-positive pids are never alive")
	}
}
