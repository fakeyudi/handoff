package bundle_test

import (
	"strings"
	"testing"
	"time"

	"pgregory.net/rapid"

	"github.com/fakeyudi/handoff/internal/bundle"
	"github.com/fakeyudi/handoff/internal/session"
)

// generateTime produces an arbitrary time.Time value truncated to second
// precision (matches JSON round-trip fidelity via RFC3339).
func generateTime(t *rapid.T, label string) time.Time {
	sec := rapid.Int64Range(1_000_000_000, 1_700_000_000).Draw(t, label+"_unix_sec")
	return time.Unix(sec, 0).UTC()
}

// generateBundle produces a fully-populated *bundle.ContextBundle with at
// least one entry in every collection field.
func generateBundle(t *rapid.T) *bundle.ContextBundle {
	// SessionMeta
	startTime := generateTime(t, "start")
	stopTime := generateTime(t, "stop")
	meta := bundle.SessionMeta{
		ID:        rapid.StringN(1, 36, -1).Draw(t, "session_id"),
		StartTime: startTime,
		StopTime:  stopTime,
		WorkDir:   rapid.StringN(1, 50, -1).Draw(t, "work_dir"),
		Duration:  rapid.StringN(1, 20, -1).Draw(t, "duration"),
	}

	// At least 1 annotation
	numAnnotations := rapid.IntRange(1, 5).Draw(t, "num_annotations")
	annotations := make([]session.Annotation, numAnnotations)
	for i := range annotations {
		annotations[i] = session.Annotation{
			Timestamp: generateTime(t, "ann_ts"),
			Message:   rapid.StringN(1, 50, -1).Draw(t, "ann_msg"),
			IsSummary: rapid.Bool().Draw(t, "ann_is_summary"),
		}
	}

	// At least 1 file edit
	numFileEdits := rapid.IntRange(1, 5).Draw(t, "num_file_edits")
	fileEdits := make([]session.FileEdit, numFileEdits)
	for i := range fileEdits {
		fileEdits[i] = session.FileEdit{
			Path:      rapid.StringN(1, 50, -1).Draw(t, "fe_path"),
			Timestamp: generateTime(t, "fe_ts"),
		}
	}

	// Non-nil GitInfo with non-empty fields
	git := &bundle.GitInfo{
		Branch:     rapid.StringN(1, 50, -1).Draw(t, "git_branch"),
		HeadCommit: rapid.StringN(1, 40, -1).Draw(t, "git_head"),
		Diff:       rapid.StringN(1, 50, -1).Draw(t, "git_diff"),
		StagedDiff: rapid.StringN(1, 50, -1).Draw(t, "git_staged"),
		RecentLog:  []string{rapid.StringN(1, 50, -1).Draw(t, "git_log")},
	}

	// At least 1 command
	numCommands := rapid.IntRange(1, 5).Draw(t, "num_commands")
	commands := make([]bundle.Command, numCommands)
	for i := range commands {
		commands[i] = bundle.Command{
			Raw:       rapid.StringN(1, 50, -1).Draw(t, "cmd_raw"),
			Timestamp: generateTime(t, "cmd_ts"),
		}
	}

	// At least 1 editor tab
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

// Feature: handoff, Property 7: Bundle completeness
func TestBundleCompleteness(t *testing.T) {
	mdRenderer := &bundle.MarkdownRenderer{}
	jsonRenderer := &bundle.JSONRenderer{}

	rapid.Check(t, func(t *rapid.T) {
		b := generateBundle(t)

		// --- Markdown ---
		mdBytes, err := mdRenderer.Render(b)
		if err != nil {
			t.Fatalf("MarkdownRenderer.Render: %v", err)
		}
		md := string(mdBytes)

		mdSections := []string{
			"## Summary",
			"## Annotations",
			"## File Edits",
			"## Git Changes",
			"## Terminal Commands",
			"## Editor Tabs",
		}
		for _, section := range mdSections {
			if !strings.Contains(md, section) {
				t.Errorf("Markdown output missing section %q", section)
			}
		}

		// --- JSON ---
		jsonBytes, err := jsonRenderer.Render(b)
		if err != nil {
			t.Fatalf("JSONRenderer.Render: %v", err)
		}
		js := string(jsonBytes)

		jsonKeys := []string{
			`"session"`,
			`"annotations"`,
			`"file_edits"`,
			`"git"`,
			`"commands"`,
			`"editor_tabs"`,
		}
		for _, key := range jsonKeys {
			if !strings.Contains(js, key) {
				t.Errorf("JSON output missing key %q", key)
			}
		}
	})
}

// Feature: handoff, Property 8: JSON bundle round-trip
func TestJSONBundleRoundTrip(t *testing.T) {
	renderer := &bundle.JSONRenderer{}
	parser := &bundle.JSONParser{}

	rapid.Check(t, func(t *rapid.T) {
		original := generateBundle(t)

		data, err := renderer.Render(original)
		if err != nil {
			t.Fatalf("JSONRenderer.Render: %v", err)
		}

		got, err := parser.Parse(data)
		if err != nil {
			t.Fatalf("JSONParser.Parse: %v", err)
		}

		// Compare each field for deep equality.
		if got.Session != original.Session {
			t.Errorf("Session mismatch: got %+v, want %+v", got.Session, original.Session)
		}
		if len(got.Annotations) != len(original.Annotations) {
			t.Fatalf("Annotations length mismatch: got %d, want %d", len(got.Annotations), len(original.Annotations))
		}
		for i := range original.Annotations {
			if got.Annotations[i] != original.Annotations[i] {
				t.Errorf("Annotations[%d] mismatch: got %+v, want %+v", i, got.Annotations[i], original.Annotations[i])
			}
		}
		if len(got.FileEdits) != len(original.FileEdits) {
			t.Fatalf("FileEdits length mismatch: got %d, want %d", len(got.FileEdits), len(original.FileEdits))
		}
		for i := range original.FileEdits {
			if got.FileEdits[i] != original.FileEdits[i] {
				t.Errorf("FileEdits[%d] mismatch: got %+v, want %+v", i, got.FileEdits[i], original.FileEdits[i])
			}
		}
		if (got.Git == nil) != (original.Git == nil) {
			t.Fatalf("Git nil mismatch: got %v, want %v", got.Git == nil, original.Git == nil)
		}
		if original.Git != nil {
			g, o := got.Git, original.Git
			if g.Branch != o.Branch || g.HeadCommit != o.HeadCommit || g.Diff != o.Diff || g.StagedDiff != o.StagedDiff {
				t.Errorf("Git mismatch: got %+v, want %+v", g, o)
			}
			if len(g.RecentLog) != len(o.RecentLog) {
				t.Fatalf("Git.RecentLog length mismatch: got %d, want %d", len(g.RecentLog), len(o.RecentLog))
			}
			for i := range o.RecentLog {
				if g.RecentLog[i] != o.RecentLog[i] {
					t.Errorf("Git.RecentLog[%d] mismatch: got %q, want %q", i, g.RecentLog[i], o.RecentLog[i])
				}
			}
		}
		if len(got.Commands) != len(original.Commands) {
			t.Fatalf("Commands length mismatch: got %d, want %d", len(got.Commands), len(original.Commands))
		}
		for i := range original.Commands {
			if got.Commands[i] != original.Commands[i] {
				t.Errorf("Commands[%d] mismatch: got %+v, want %+v", i, got.Commands[i], original.Commands[i])
			}
		}
		if len(got.EditorTabs) != len(original.EditorTabs) {
			t.Fatalf("EditorTabs length mismatch: got %d, want %d", len(got.EditorTabs), len(original.EditorTabs))
		}
		for i := range original.EditorTabs {
			if got.EditorTabs[i] != original.EditorTabs[i] {
				t.Errorf("EditorTabs[%d] mismatch: got %q, want %q", i, got.EditorTabs[i], original.EditorTabs[i])
			}
		}
	})
}

// Feature: handoff, Property 9: Markdown bundle round-trip
func TestMarkdownBundleRoundTrip(t *testing.T) {
	renderer := &bundle.MarkdownRenderer{}
	parser := &bundle.MarkdownParser{}

	rapid.Check(t, func(t *rapid.T) {
		original := generateBundle(t)

		data, err := renderer.Render(original)
		if err != nil {
			t.Fatalf("MarkdownRenderer.Render: %v", err)
		}

		got, err := parser.Parse(data)
		if err != nil {
			t.Fatalf("MarkdownParser.Parse: %v", err)
		}

		// Compare each field for deep equality.
		if got.Session != original.Session {
			t.Errorf("Session mismatch: got %+v, want %+v", got.Session, original.Session)
		}
		if len(got.Annotations) != len(original.Annotations) {
			t.Fatalf("Annotations length mismatch: got %d, want %d", len(got.Annotations), len(original.Annotations))
		}
		for i := range original.Annotations {
			if got.Annotations[i] != original.Annotations[i] {
				t.Errorf("Annotations[%d] mismatch: got %+v, want %+v", i, got.Annotations[i], original.Annotations[i])
			}
		}
		if len(got.FileEdits) != len(original.FileEdits) {
			t.Fatalf("FileEdits length mismatch: got %d, want %d", len(got.FileEdits), len(original.FileEdits))
		}
		for i := range original.FileEdits {
			if got.FileEdits[i] != original.FileEdits[i] {
				t.Errorf("FileEdits[%d] mismatch: got %+v, want %+v", i, got.FileEdits[i], original.FileEdits[i])
			}
		}
		if (got.Git == nil) != (original.Git == nil) {
			t.Fatalf("Git nil mismatch: got %v, want %v", got.Git == nil, original.Git == nil)
		}
		if original.Git != nil {
			g, o := got.Git, original.Git
			if g.Branch != o.Branch || g.HeadCommit != o.HeadCommit || g.Diff != o.Diff || g.StagedDiff != o.StagedDiff {
				t.Errorf("Git mismatch: got %+v, want %+v", g, o)
			}
			if len(g.RecentLog) != len(o.RecentLog) {
				t.Fatalf("Git.RecentLog length mismatch: got %d, want %d", len(g.RecentLog), len(o.RecentLog))
			}
			for i := range o.RecentLog {
				if g.RecentLog[i] != o.RecentLog[i] {
					t.Errorf("Git.RecentLog[%d] mismatch: got %q, want %q", i, g.RecentLog[i], o.RecentLog[i])
				}
			}
		}
		if len(got.Commands) != len(original.Commands) {
			t.Fatalf("Commands length mismatch: got %d, want %d", len(got.Commands), len(original.Commands))
		}
		for i := range original.Commands {
			if got.Commands[i] != original.Commands[i] {
				t.Errorf("Commands[%d] mismatch: got %+v, want %+v", i, got.Commands[i], original.Commands[i])
			}
		}
		if len(got.EditorTabs) != len(original.EditorTabs) {
			t.Fatalf("EditorTabs length mismatch: got %d, want %d", len(got.EditorTabs), len(original.EditorTabs))
		}
		for i := range original.EditorTabs {
			if got.EditorTabs[i] != original.EditorTabs[i] {
				t.Errorf("EditorTabs[%d] mismatch: got %q, want %q", i, got.EditorTabs[i], original.EditorTabs[i])
			}
		}
	})
}
