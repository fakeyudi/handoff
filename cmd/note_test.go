package cmd

import (
	"testing"
	"time"

	"pgregory.net/rapid"

	"github.com/fakeyudi/handoff/internal/session"
)

// Feature: handoff, Property 6: Annotation persistence
func TestAnnotationPersistence(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		// Generate a non-empty message.
		msg := rapid.StringMatching(`[^\x00]{1,200}`).Draw(rt, "message")

		tmp := t.TempDir()
		t.Setenv("XDG_DATA_HOME", tmp)

		store, err := session.NewSessionStore()
		if err != nil {
			rt.Fatalf("NewSessionStore: %v", err)
		}

		// Seed a base session.
		base := &session.Session{
			ID:          "test-id",
			StartTime:   time.Now(),
			WorkDir:     tmp,
			Annotations: []session.Annotation{},
			FileEdits:   []session.FileEdit{},
		}
		if err := store.Save(base); err != nil {
			rt.Fatalf("Save base session: %v", err)
		}

		// --- note path (IsSummary = false) ---
		before := time.Now()
		s, err := store.Load()
		if err != nil {
			rt.Fatalf("Load: %v", err)
		}
		s.Annotations = append(s.Annotations, session.Annotation{
			Timestamp: time.Now(),
			Message:   msg,
			IsSummary: false,
		})
		if err := store.Save(s); err != nil {
			rt.Fatalf("Save after note: %v", err)
		}
		after := time.Now()

		loaded, err := store.Load()
		if err != nil {
			rt.Fatalf("Load after note: %v", err)
		}
		if len(loaded.Annotations) == 0 {
			rt.Fatal("expected at least one annotation after note, got none")
		}
		ann := loaded.Annotations[len(loaded.Annotations)-1]
		if ann.Message != msg {
			rt.Errorf("note: message mismatch: got %q, want %q", ann.Message, msg)
		}
		if ann.Timestamp.IsZero() {
			rt.Error("note: timestamp is zero")
		}
		if ann.Timestamp.Before(before) || ann.Timestamp.After(after) {
			rt.Errorf("note: timestamp %v outside expected range [%v, %v]", ann.Timestamp, before, after)
		}
		if ann.IsSummary {
			rt.Error("note: IsSummary should be false")
		}

		// --- stop -m path (IsSummary = true) ---
		// Reset to a fresh session.
		if err := store.Save(base); err != nil {
			rt.Fatalf("Save base session (reset): %v", err)
		}

		before2 := time.Now()
		s2, err := store.Load()
		if err != nil {
			rt.Fatalf("Load for stop -m: %v", err)
		}
		now := time.Now()
		s2.Annotations = append(s2.Annotations, session.Annotation{
			Timestamp: now,
			Message:   msg,
			IsSummary: true,
		})
		if err := store.Save(s2); err != nil {
			rt.Fatalf("Save after stop -m: %v", err)
		}
		after2 := time.Now()

		loaded2, err := store.Load()
		if err != nil {
			rt.Fatalf("Load after stop -m: %v", err)
		}
		if len(loaded2.Annotations) == 0 {
			rt.Fatal("expected at least one annotation after stop -m, got none")
		}
		ann2 := loaded2.Annotations[len(loaded2.Annotations)-1]
		if ann2.Message != msg {
			rt.Errorf("stop -m: message mismatch: got %q, want %q", ann2.Message, msg)
		}
		if ann2.Timestamp.IsZero() {
			rt.Error("stop -m: timestamp is zero")
		}
		if ann2.Timestamp.Before(before2) || ann2.Timestamp.After(after2) {
			rt.Errorf("stop -m: timestamp %v outside expected range [%v, %v]", ann2.Timestamp, before2, after2)
		}
		if !ann2.IsSummary {
			rt.Error("stop -m: IsSummary should be true")
		}
	})
}
