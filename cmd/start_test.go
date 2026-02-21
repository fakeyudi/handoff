package cmd

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"
	"github.com/fakeyudi/handoff/internal/session"
)

// executeCommand runs a cobra command with the given args and captures combined output.
func executeCommand(root *cobra.Command, args ...string) (output string, err error) {
	buf := new(bytes.Buffer)
	root.SetOut(buf)
	root.SetErr(buf)
	root.SetArgs(args)
	_, err = root.ExecuteC()
	return buf.String(), err
}

// TestDoubleStartError verifies that running "start" when a session is already
// active returns an error containing "session already in progress".
func TestDoubleStartError(t *testing.T) {
	// Point XDG_DATA_HOME at a temp dir so we don't touch real state.
	tmp := t.TempDir()
	t.Setenv("XDG_DATA_HOME", tmp)

	// Pre-create a session on disk to simulate an already-active session.
	store, err := session.NewSessionStore()
	if err != nil {
		t.Fatalf("NewSessionStore: %v", err)
	}
	existing := &session.Session{
		ID:          "test-id",
		StartTime:   time.Now(),
		WorkDir:     tmp,
		Annotations: []session.Annotation{},
		FileEdits:   []session.FileEdit{},
	}
	if err := store.Save(existing); err != nil {
		t.Fatalf("Save: %v", err)
	}

	// Reset cobra state between runs.
	rootCmd.ResetFlags()

	out, err := executeCommand(rootCmd, "start")
	if err == nil {
		t.Fatal("expected an error from double-start, got nil")
	}
	combined := out + err.Error()
	if !strings.Contains(combined, "session already in progress") {
		t.Errorf("expected error to contain %q, got: %q", "session already in progress", combined)
	}
}
