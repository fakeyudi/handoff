package collector

import (
	"context"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/fakeyudi/handoff/internal/session"
)

// exitCode128Error returns a real *exec.ExitError with exit code 128
// by running a shell command that exits with that code.
func exitCode128Error() error {
	cmd := exec.Command("sh", "-c", "exit 128")
	return cmd.Run()
}

// TestGitCollectorNonGitRepo verifies that when the working directory is not a
// git repository (runner returns exit code 128), Collect returns nil GitInfo
// and a warning containing "not a git repository".
func TestGitCollectorNonGitRepo(t *testing.T) {
	exitErr := exitCode128Error()
	if exitErr == nil {
		t.Fatal("expected exit code 128 error, got nil")
	}

	mockRunner := func(workDir string, args ...string) (string, error) {
		return "", exitErr
	}

	sess := &session.Session{
		ID:        "test-session",
		StartTime: time.Now().Add(-1 * time.Hour),
		WorkDir:   "/some/dir",
	}

	gc := &GitCollector{
		WorkDir: "/some/dir",
		Runner:  mockRunner,
	}

	result, err := gc.Collect(context.Background(), sess)
	if err != nil {
		t.Fatalf("Collect returned unexpected error: %v", err)
	}
	if result.GitInfo != nil {
		t.Errorf("expected GitInfo to be nil for non-git repo, got %+v", result.GitInfo)
	}

	found := false
	for _, w := range result.Warnings {
		if strings.Contains(w, "not a git repository") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected warning containing 'not a git repository', got: %v", result.Warnings)
	}
}

// TestGitCollectorSuccess verifies that when all git commands succeed, Collect
// populates all GitInfo fields correctly.
func TestGitCollectorSuccess(t *testing.T) {
	responses := map[string]string{
		"rev-parse --abbrev-ref HEAD": "main\n",
		"rev-parse HEAD":              "abc123def456\n",
		"diff":                        "diff --git a/foo.go b/foo.go\n--- a/foo.go\n+++ b/foo.go\n",
		"diff --staged":               "diff --git a/bar.go b/bar.go\n--- a/bar.go\n+++ b/bar.go\n",
		"log --oneline":               "abc123 first commit\ndef456 second commit\n",
	}

	mockRunner := func(workDir string, args ...string) (string, error) {
		key := strings.Join(args, " ")
		// log command includes a --since=... flag; match by prefix
		if strings.HasPrefix(key, "log --oneline") {
			return responses["log --oneline"], nil
		}
		if out, ok := responses[key]; ok {
			return out, nil
		}
		t.Errorf("unexpected git command: %q", key)
		return "", nil
	}

	sess := &session.Session{
		ID:        "test-session",
		StartTime: time.Now().Add(-2 * time.Hour),
		WorkDir:   "/repo",
	}

	gc := &GitCollector{
		WorkDir: "/repo",
		Runner:  mockRunner,
	}

	result, err := gc.Collect(context.Background(), sess)
	if err != nil {
		t.Fatalf("Collect returned unexpected error: %v", err)
	}
	if len(result.Warnings) != 0 {
		t.Errorf("expected no warnings, got: %v", result.Warnings)
	}
	if result.GitInfo == nil {
		t.Fatal("expected GitInfo to be populated, got nil")
	}

	gi := result.GitInfo
	if gi.Branch != "main" {
		t.Errorf("expected Branch %q, got %q", "main", gi.Branch)
	}
	if gi.HeadCommit != "abc123def456" {
		t.Errorf("expected HeadCommit %q, got %q", "abc123def456", gi.HeadCommit)
	}
	if gi.Diff == "" {
		t.Error("expected Diff to be non-empty")
	}
	if gi.StagedDiff == "" {
		t.Error("expected StagedDiff to be non-empty")
	}
	if len(gi.RecentLog) != 2 {
		t.Errorf("expected 2 log entries, got %d: %v", len(gi.RecentLog), gi.RecentLog)
	}
	if gi.RecentLog[0] != "abc123 first commit" {
		t.Errorf("expected first log entry %q, got %q", "abc123 first commit", gi.RecentLog[0])
	}
	if gi.RecentLog[1] != "def456 second commit" {
		t.Errorf("expected second log entry %q, got %q", "def456 second commit", gi.RecentLog[1])
	}
}
