package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"pgregory.net/rapid"

	"github.com/fakeyudi/handoff/internal/bundle"
	"github.com/fakeyudi/handoff/internal/session"
)

// capturePrintBundle redirects os.Stdout while calling printBundle and returns
// the captured output as a string.
func capturePrintBundle(b *bundle.ContextBundle) (string, error) {
	// Save original stdout.
	origStdout := os.Stdout

	r, w, err := os.Pipe()
	if err != nil {
		return "", err
	}
	os.Stdout = w

	printBundle(b)

	// Close the write end so the read below doesn't block.
	w.Close()
	os.Stdout = origStdout

	buf := new(strings.Builder)
	tmp := make([]byte, 4096)
	for {
		n, readErr := r.Read(tmp)
		if n > 0 {
			buf.Write(tmp[:n])
		}
		if readErr != nil {
			break
		}
	}
	r.Close()

	return buf.String(), nil
}

// generateViewBundle produces a fully-populated *bundle.ContextBundle suitable
// for testing the view command's section ordering.
func generateViewBundle(t *rapid.T) *bundle.ContextBundle {
	sec := rapid.Int64Range(1_000_000_000, 1_700_000_000).Draw(t, "unix_sec")
	ts := time.Unix(sec, 0).UTC()

	meta := bundle.SessionMeta{
		ID:        rapid.StringN(1, 36, -1).Draw(t, "session_id"),
		StartTime: ts,
		StopTime:  ts,
		WorkDir:   rapid.StringN(1, 50, -1).Draw(t, "work_dir"),
		Duration:  rapid.StringN(1, 20, -1).Draw(t, "duration"),
	}

	numAnnotations := rapid.IntRange(1, 5).Draw(t, "num_annotations")
	annotations := make([]session.Annotation, numAnnotations)
	for i := range annotations {
		annotations[i] = session.Annotation{
			Timestamp: ts,
			Message:   rapid.StringN(1, 50, -1).Draw(t, "ann_msg"),
			IsSummary: rapid.Bool().Draw(t, "ann_is_summary"),
		}
	}

	numFileEdits := rapid.IntRange(1, 5).Draw(t, "num_file_edits")
	fileEdits := make([]session.FileEdit, numFileEdits)
	for i := range fileEdits {
		fileEdits[i] = session.FileEdit{
			Path:      rapid.StringN(1, 50, -1).Draw(t, "fe_path"),
			Timestamp: ts,
		}
	}

	git := &bundle.GitInfo{
		Branch:     rapid.StringN(1, 30, -1).Draw(t, "git_branch"),
		HeadCommit: rapid.StringN(1, 40, -1).Draw(t, "git_head"),
		Diff:       rapid.StringN(1, 50, -1).Draw(t, "git_diff"),
		StagedDiff: rapid.StringN(1, 50, -1).Draw(t, "git_staged"),
		RecentLog:  []string{rapid.StringN(1, 50, -1).Draw(t, "git_log")},
	}

	numCommands := rapid.IntRange(1, 5).Draw(t, "num_commands")
	commands := make([]bundle.Command, numCommands)
	for i := range commands {
		commands[i] = bundle.Command{
			Raw:       rapid.StringN(1, 50, -1).Draw(t, "cmd_raw"),
			Timestamp: ts,
		}
	}

	numTabs := rapid.IntRange(1, 5).Draw(t, "num_tabs")
	tabs := make([]string, numTabs)
	for i := range tabs {
		tabs[i] = rapid.StringN(1, 50, -1).Draw(t, "tab_path")
	}

	return &bundle.ContextBundle{
		Session:     meta,
		Annotations: annotations,
		FileEdits:   fileEdits,
		Git:         git,
		Commands:    commands,
		EditorTabs:  tabs,
	}
}

// TestViewNonExistentFile verifies that viewing a missing file returns
// "file not found: <path>".
func TestViewNonExistentFile(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_DATA_HOME", tmp)

	missingPath := filepath.Join(tmp, "does-not-exist.md")

	rootCmd.ResetFlags()
	out, err := executeCommand(rootCmd, "view", missingPath)
	if err == nil {
		t.Fatal("expected an error for non-existent file, got nil")
	}
	combined := out + err.Error()
	expected := "file not found: " + missingPath
	if !strings.Contains(combined, expected) {
		t.Errorf("expected error to contain %q, got: %q", expected, combined)
	}
}

// TestViewInvalidBundle verifies that viewing a file without the handoff
// sentinel returns "not a valid handoff bundle".
func TestViewInvalidBundle(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_DATA_HOME", tmp)

	// Write a plain Markdown file with no handoff sentinel.
	plainMD := filepath.Join(tmp, "plain.md")
	if err := os.WriteFile(plainMD, []byte("# Just a regular markdown file\n\nNo sentinel here.\n"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	rootCmd.ResetFlags()
	out, err := executeCommand(rootCmd, "view", plainMD)
	if err == nil {
		t.Fatal("expected an error for invalid bundle, got nil")
	}
	combined := out + err.Error()
	if !strings.Contains(combined, "not a valid handoff bundle") {
		t.Errorf("expected error to contain %q, got: %q", "not a valid handoff bundle", combined)
	}
}

// Feature: handoff, Property 12: View section order
func TestViewSectionOrder(t *testing.T) {
	// The required section order as printed by printBundle.
	sectionHeaders := []string{
		"## Summary",
		"## Annotations",
		"## File Edits",
		"## Git Changes",
		"## Terminal Commands",
		"## Editor Tabs",
	}

	rapid.Check(t, func(rt *rapid.T) {
		b := generateViewBundle(rt)

		output, err := capturePrintBundle(b)
		if err != nil {
			rt.Fatalf("capturePrintBundle: %v", err)
		}

		// Find the index of each section header in the output string.
		positions := make([]int, len(sectionHeaders))
		for i, header := range sectionHeaders {
			pos := strings.Index(output, header)
			if pos == -1 {
				rt.Fatalf("section header %q not found in output:\n%s", header, output)
			}
			positions[i] = pos
		}

		// Assert each section appears strictly before all subsequent sections.
		for i := 0; i < len(positions)-1; i++ {
			if positions[i] >= positions[i+1] {
				rt.Errorf(
					"section %q (pos %d) does not appear before %q (pos %d) in output:\n%s",
					sectionHeaders[i], positions[i],
					sectionHeaders[i+1], positions[i+1],
					output,
				)
			}
		}
	})
}
