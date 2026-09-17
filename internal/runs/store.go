// Package runs keeps the record of every run and the log of every step on
// disk, so the editor and the command line can inspect runs after the fact.
//
// Layout under Root: <run-id>/run.json holds the record, and
// <run-id>/logs/<step-id>.log holds what that step did.
package runs

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"syscall"
	"time"

	"github.com/Gabes-Tint/mega-agents/internal/engine"
	"github.com/Gabes-Tint/mega-agents/internal/megahome"
)

// Interrupted marks a run, and its unfinished steps, whose process ended
// before the run did.
const Interrupted engine.Status = "interrupted"

// Cancelled marks a run someone stopped.
const Cancelled engine.Status = "cancelled"

var ErrNotFound = errors.New("not found")

// identifier restricts run and step ids to one safe path segment.
var identifier = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]*$`)

type Record struct {
	ID         string        `json:"id"`
	Workflow   string        `json:"workflow"`
	Status     engine.Status `json:"status"`
	StartedAt  string        `json:"startedAt"`
	FinishedAt string        `json:"finishedAt,omitempty"`
	// PID is the process executing the run, used to tell a live run from
	// one whose process died.
	PID   int           `json:"pid"`
	Steps []engine.Step `json:"steps"`
}

type Store struct {
	Root string
	// Now is the clock run ids and start times come from; nil is the system
	// clock.
	Now func() time.Time
}

// DefaultStore keeps runs under the Mega Agents home.
func DefaultStore() (*Store, error) {
	home, err := megahome.Dir()
	if err != nil {
		return nil, err
	}
	return &Store{Root: filepath.Join(home, "runs")}, nil
}

func (store *Store) now() time.Time {
	if store.Now != nil {
		return store.Now().UTC()
	}
	return time.Now().UTC()
}

// Create records a new running run of the workflow with its planned steps.
func (store *Store) Create(workflow string, steps []engine.Step) (Record, error) {
	suffix := make([]byte, 3)
	if _, err := rand.Read(suffix); err != nil {
		return Record{}, fmt.Errorf("cannot create a run id: %w", err)
	}
	started := store.now()
	record := Record{
		ID:       started.Format("20060102T150405Z") + "-" + hex.EncodeToString(suffix),
		Workflow: workflow, Status: engine.Running, StartedAt: started.Format(time.RFC3339),
		PID: os.Getpid(), Steps: steps,
	}
	if err := os.MkdirAll(filepath.Join(store.Root, record.ID, "logs"), 0o750); err != nil {
		return Record{}, fmt.Errorf("cannot create the run folder: %w", err)
	}
	return record, store.Save(record)
}

// Save replaces the record atomically, so readers never see half a file.
func (store *Store) Save(record Record) error {
	if !identifier.MatchString(record.ID) {
		return fmt.Errorf("invalid run id %q", record.ID)
	}
	data, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		return fmt.Errorf("cannot encode run %s: %w", record.ID, err)
	}
	dir := filepath.Join(store.Root, record.ID)
	temporary, err := os.CreateTemp(dir, "run-*.json")
	if err != nil {
		return fmt.Errorf("cannot save run %s: %w", record.ID, err)
	}
	defer os.Remove(temporary.Name())
	if _, err := temporary.Write(append(data, '\n')); err != nil {
		temporary.Close()
		return fmt.Errorf("cannot save run %s: %w", record.ID, err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("cannot save run %s: %w", record.ID, err)
	}
	if err := os.Rename(temporary.Name(), filepath.Join(dir, "run.json")); err != nil {
		return fmt.Errorf("cannot save run %s: %w", record.ID, err)
	}
	return nil
}

// Load reads a run. A run still marked running whose process has ended is
// reported as interrupted.
func (store *Store) Load(id string) (Record, error) {
	if !identifier.MatchString(id) {
		return Record{}, ErrNotFound
	}
	data, err := os.ReadFile(filepath.Join(store.Root, id, "run.json"))
	if errors.Is(err, fs.ErrNotExist) {
		return Record{}, ErrNotFound
	}
	if err != nil {
		return Record{}, fmt.Errorf("cannot read run %s: %w", id, err)
	}
	var record Record
	if err := json.Unmarshal(data, &record); err != nil {
		return Record{}, fmt.Errorf("run %s has an unreadable record: %w", id, err)
	}
	if record.Status == engine.Running && !processAlive(record.PID) {
		record.Status = Interrupted
		for i, step := range record.Steps {
			if step.Status == engine.Running || step.Status == engine.Pending {
				record.Steps[i].Status = Interrupted
			}
		}
	}
	return record, nil
}

// List returns every run, newest first.
func (store *Store) List() ([]Record, error) {
	entries, err := os.ReadDir(store.Root)
	if errors.Is(err, fs.ErrNotExist) {
		return []Record{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("cannot list runs: %w", err)
	}
	records := []Record{}
	for _, entry := range slices.Backward(entries) {
		if !entry.IsDir() {
			continue
		}
		record, err := store.Load(entry.Name())
		if errors.Is(err, ErrNotFound) {
			continue
		}
		if err != nil {
			return nil, err
		}
		records = append(records, record)
	}
	return records, nil
}

// OpenLog opens a step's log for appending.
func (store *Store) OpenLog(runID string, stepID string) (io.WriteCloser, error) {
	if !identifier.MatchString(runID) || !identifier.MatchString(stepID) {
		return nil, fmt.Errorf("invalid run or step id %q/%q", runID, stepID)
	}
	path := filepath.Join(store.Root, runID, "logs", stepID+".log")
	file, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return nil, fmt.Errorf("cannot open the log of %s: %w", stepID, err)
	}
	return file, nil
}

// ReadLog returns everything a step logged.
func (store *Store) ReadLog(runID string, stepID string) (string, error) {
	if !identifier.MatchString(runID) || !identifier.MatchString(stepID) {
		return "", ErrNotFound
	}
	data, err := os.ReadFile(filepath.Join(store.Root, runID, "logs", stepID+".log"))
	if errors.Is(err, fs.ErrNotExist) {
		return "", ErrNotFound
	}
	if err != nil {
		return "", fmt.Errorf("cannot read the log of %s: %w", stepID, err)
	}
	return string(data), nil
}

func processAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	if pid == os.Getpid() {
		return true
	}
	err := syscall.Kill(pid, 0)
	return err == nil || errors.Is(err, syscall.EPERM)
}
