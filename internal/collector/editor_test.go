package collector

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/fakeyudi/handoff/internal/session"
)

// TestEditorCollectorWithFixture verifies that Collect correctly reads
// workspace folders from VS Code workspace.json files.
func TestEditorCollectorWithFixture(t *testing.T) {
	tmpDir := t.TempDir()

	// Create two fake workspace storage entries with workspace.json files.
	workspaces := []struct {
		hash   string
		folder string
	}{
		{"abc123", "file:///home/user/project"},
		{"def456", "file:///home/user/other-project"},
	}

	for _, ws := range workspaces {
		wsDir := filepath.Join(tmpDir, ws.hash)
		if err := os.MkdirAll(wsDir, 0755); err != nil {
			t.Fatalf("failed to create workspace dir: %v", err)
		}
		content := fmt.Sprintf(`{"folder": %q}`, ws.folder)
		if err := os.WriteFile(filepath.Join(wsDir, "workspace.json"), []byte(content), 0644); err != nil {
			t.Fatalf("failed to write workspace.json: %v", err)
		}
	}

	ec := &EditorCollector{StateDir: tmpDir}
	sess := &session.Session{
		ID:        "test-session",
		StartTime: time.Now().Add(-1 * time.Hour),
		WorkDir:   "/home/user/project",
	}

	result, err := ec.Collect(context.Background(), sess)
	if err != nil {
		t.Fatalf("Collect returned unexpected error: %v", err)
	}

	// Only the tab under WorkDir should be included.
	want := map[string]bool{
		"/home/user/project": true,
	}

	if len(result.EditorTabs) != len(want) {
		t.Errorf("expected %d editor tabs, got %d: %v", len(want), len(result.EditorTabs), result.EditorTabs)
	}

	for _, tab := range result.EditorTabs {
		if !want[tab] {
			t.Errorf("unexpected editor tab: %q", tab)
		}
	}
}

// TestEditorCollectorMissingStateDir verifies that when the VS Code storage
// directory does not exist, Collect returns an empty EditorTabs and a warning.
func TestEditorCollectorMissingStateDir(t *testing.T) {
	ec := &EditorCollector{StateDir: "/nonexistent/path/that/does/not/exist"}
	sess := &session.Session{
		ID:        "test-session",
		StartTime: time.Now().Add(-1 * time.Hour),
		WorkDir:   "/some/dir",
	}

	result, err := ec.Collect(context.Background(), sess)
	if err != nil {
		t.Fatalf("Collect returned unexpected error: %v", err)
	}

	if len(result.EditorTabs) != 0 {
		t.Errorf("expected empty EditorTabs, got: %v", result.EditorTabs)
	}

	if len(result.Warnings) == 0 {
		t.Error("expected at least one warning for missing state dir, got none")
	}
}
