package cmd

import (
	"strings"
	"testing"
)

// TestStopNoSessionError verifies that running "stop" when no session is active
// returns an error containing "no active session".
func TestStopNoSessionError(t *testing.T) {
	// Point XDG_DATA_HOME at an empty temp dir so no session file exists.
	tmp := t.TempDir()
	t.Setenv("XDG_DATA_HOME", tmp)

	// Reset cobra state between runs.
	rootCmd.ResetFlags()

	out, err := executeCommand(rootCmd, "stop")
	if err == nil {
		t.Fatal("expected an error from stop with no session, got nil")
	}
	combined := out + err.Error()
	if !strings.Contains(combined, "no active session") {
		t.Errorf("expected error to contain %q, got: %q", "no active session", combined)
	}
}
