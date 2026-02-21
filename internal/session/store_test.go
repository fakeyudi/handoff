package session_test

import (
	"errors"
	"os"
	"testing"
	"time"

	"pgregory.net/rapid"

	"github.com/fakeyudi/handoff/internal/session"
)

// generateTime produces an arbitrary time.Time value.
// We truncate to second precision to match JSON round-trip fidelity
// (time.Time marshals to RFC3339 which has second precision by default).
func generateTime(t *rapid.T) time.Time {
	sec := rapid.Int64Range(0, 1_700_000_000).Draw(t, "unix_sec")
	return time.Unix(sec, 0).UTC()
}

// generateAnnotation produces an arbitrary Annotation.
func generateAnnotation(t *rapid.T, label string) session.Annotation {
	return session.Annotation{
		Timestamp: generateTime(t),
		Message:   rapid.StringN(1, 200, -1).Draw(t, label+"_msg"),
		IsSummary: rapid.Bool().Draw(t, label+"_is_summary"),
	}
}

// generateFileEdit produces an arbitrary FileEdit.
func generateFileEdit(t *rapid.T, label string) session.FileEdit {
	return session.FileEdit{
		Path:      rapid.StringN(1, 100, -1).Draw(t, label+"_path"),
		Timestamp: generateTime(t),
	}
}

// generateSession produces an arbitrary Session value.
func generateSession(t *rapid.T) *session.Session {
	id := rapid.StringN(1, 36, -1).Draw(t, "id")
	startTime := generateTime(t)
	workDir := rapid.StringN(1, 100, -1).Draw(t, "work_dir")

	var stopTime *time.Time
	if rapid.Bool().Draw(t, "has_stop_time") {
		st := generateTime(t)
		stopTime = &st
	}

	numAnnotations := rapid.IntRange(0, 5).Draw(t, "num_annotations")
	annotations := make([]session.Annotation, numAnnotations)
	for i := range annotations {
		annotations[i] = generateAnnotation(t, "annotation")
	}

	numFileEdits := rapid.IntRange(0, 5).Draw(t, "num_file_edits")
	fileEdits := make([]session.FileEdit, numFileEdits)
	for i := range fileEdits {
		fileEdits[i] = generateFileEdit(t, "file_edit")
	}

	return &session.Session{
		ID:          id,
		StartTime:   startTime,
		StopTime:    stopTime,
		WorkDir:     workDir,
		Annotations: annotations,
		FileEdits:   fileEdits,
	}
}

// Feature: handoff, Property 1: Session persistence round-trip
func TestSessionPersistenceRoundTrip(t *testing.T) {
	// Point the store at a temp directory via XDG_DATA_HOME.
	// Use the outer *testing.T for TempDir/Setenv (rapid.T doesn't have these).
	tmp := t.TempDir()
	t.Setenv("XDG_DATA_HOME", tmp)

	store, err := session.NewSessionStore()
	if err != nil {
		t.Fatalf("NewSessionStore: %v", err)
	}

	rapid.Check(t, func(t *rapid.T) {
		original := generateSession(t)

		if err := store.Save(original); err != nil {
			t.Fatalf("Save: %v", err)
		}

		loaded, err := store.Load()
		if err != nil {
			t.Fatalf("Load: %v", err)
		}

		// Compare fields individually to produce clear failure messages.
		if loaded.ID != original.ID {
			t.Errorf("ID mismatch: got %q, want %q", loaded.ID, original.ID)
		}
		if !loaded.StartTime.Equal(original.StartTime) {
			t.Errorf("StartTime mismatch: got %v, want %v", loaded.StartTime, original.StartTime)
		}
		if loaded.WorkDir != original.WorkDir {
			t.Errorf("WorkDir mismatch: got %q, want %q", loaded.WorkDir, original.WorkDir)
		}

		// StopTime
		if (loaded.StopTime == nil) != (original.StopTime == nil) {
			t.Errorf("StopTime nil mismatch: got %v, want %v", loaded.StopTime, original.StopTime)
		} else if loaded.StopTime != nil && !loaded.StopTime.Equal(*original.StopTime) {
			t.Errorf("StopTime mismatch: got %v, want %v", *loaded.StopTime, *original.StopTime)
		}

		// Annotations
		if len(loaded.Annotations) != len(original.Annotations) {
			t.Fatalf("Annotations length mismatch: got %d, want %d", len(loaded.Annotations), len(original.Annotations))
		}
		for i, a := range original.Annotations {
			got := loaded.Annotations[i]
			if !got.Timestamp.Equal(a.Timestamp) {
				t.Errorf("Annotations[%d].Timestamp mismatch: got %v, want %v", i, got.Timestamp, a.Timestamp)
			}
			if got.Message != a.Message {
				t.Errorf("Annotations[%d].Message mismatch: got %q, want %q", i, got.Message, a.Message)
			}
			if got.IsSummary != a.IsSummary {
				t.Errorf("Annotations[%d].IsSummary mismatch: got %v, want %v", i, got.IsSummary, a.IsSummary)
			}
		}

		// FileEdits
		if len(loaded.FileEdits) != len(original.FileEdits) {
			t.Fatalf("FileEdits length mismatch: got %d, want %d", len(loaded.FileEdits), len(original.FileEdits))
		}
		for i, fe := range original.FileEdits {
			got := loaded.FileEdits[i]
			if got.Path != fe.Path {
				t.Errorf("FileEdits[%d].Path mismatch: got %q, want %q", i, got.Path, fe.Path)
			}
			if !got.Timestamp.Equal(fe.Timestamp) {
				t.Errorf("FileEdits[%d].Timestamp mismatch: got %v, want %v", i, got.Timestamp, fe.Timestamp)
			}
		}
	})
}

// TestLoadReturnsErrNoSession verifies that Load returns ErrNoSession when no
// session file exists on disk.
func TestLoadReturnsErrNoSession(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_DATA_HOME", tmp)

	store, err := session.NewSessionStore()
	if err != nil {
		t.Fatalf("NewSessionStore: %v", err)
	}

	_, err = store.Load()
	if err == nil {
		t.Fatal("expected ErrNoSession, got nil")
	}
	if !errors.Is(err, session.ErrNoSession) {
		t.Errorf("expected ErrNoSession, got: %v", err)
	}
}

// TestSaveFailurePropagatesError verifies that Save returns an error when the
// underlying directory is not writable.
func TestSaveFailurePropagatesError(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("running as root; permission checks are ineffective")
	}

	tmp := t.TempDir()
	// Make the directory unwritable so os.CreateTemp fails.
	if err := os.Chmod(tmp, 0o000); err != nil {
		t.Fatalf("chmod: %v", err)
	}
	// Restore permissions so TempDir cleanup can remove it.
	t.Cleanup(func() { os.Chmod(tmp, 0o755) })

	t.Setenv("XDG_DATA_HOME", tmp)

	// NewSessionStore calls os.MkdirAll on the handoff sub-dir; that will fail
	// because tmp is unreadable/unwritable, so we expect an error here.
	_, err := session.NewSessionStore()
	if err == nil {
		t.Fatal("expected error creating store in unwritable directory, got nil")
	}
}
