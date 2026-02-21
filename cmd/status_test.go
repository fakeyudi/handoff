package cmd

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"pgregory.net/rapid"

	"github.com/fakeyudi/handoff/internal/session"
)

// Feature: handoff, Property 11: Status counts accuracy
func TestStatusCountsAccuracy(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		N := rapid.IntRange(0, 20).Draw(rt, "N") // number of file edits
		M := rapid.IntRange(0, 20).Draw(rt, "M") // number of annotations

		tmp := t.TempDir()
		t.Setenv("XDG_DATA_HOME", tmp)

		store, err := session.NewSessionStore()
		if err != nil {
			rt.Fatalf("NewSessionStore: %v", err)
		}

		// Build N FileEdits.
		fileEdits := make([]session.FileEdit, N)
		for i := 0; i < N; i++ {
			fileEdits[i] = session.FileEdit{
				Path:      fmt.Sprintf("file%d.go", i),
				Timestamp: time.Now(),
			}
		}

		// Build M Annotations.
		annotations := make([]session.Annotation, M)
		for i := 0; i < M; i++ {
			annotations[i] = session.Annotation{
				Timestamp: time.Now(),
				Message:   fmt.Sprintf("annotation %d", i),
				IsSummary: false,
			}
		}

		s := &session.Session{
			ID:          "test-id",
			StartTime:   time.Now(),
			WorkDir:     tmp,
			FileEdits:   fileEdits,
			Annotations: annotations,
		}
		if err := store.Save(s); err != nil {
			rt.Fatalf("Save: %v", err)
		}

		// Run the status command and capture output.
		rootCmd.ResetFlags()
		out, err := executeCommand(rootCmd, "status")
		if err != nil {
			rt.Fatalf("status command error: %v", err)
		}

		wantFileEdits := fmt.Sprintf("File edits: %d", N)
		wantAnnotations := fmt.Sprintf("Annotations: %d", M)

		if !strings.Contains(out, wantFileEdits) {
			rt.Errorf("expected output to contain %q, got:\n%s", wantFileEdits, out)
		}
		if !strings.Contains(out, wantAnnotations) {
			rt.Errorf("expected output to contain %q, got:\n%s", wantAnnotations, out)
		}
	})
}
