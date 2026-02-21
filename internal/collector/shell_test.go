package collector

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"pgregory.net/rapid"

	"github.com/fakeyudi/handoff/internal/session"
)

// Feature: handoff, Property 4: Shell history time-window filtering
func TestShellHistoryTimeWindowFiltering(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		// Generate a session window [start, stop].
		baseEpoch := int64(1_700_000_000) // ~Nov 2023
		startOffset := rapid.Int64Range(0, 3600).Draw(t, "startOffset")
		windowDuration := rapid.Int64Range(60, 7200).Draw(t, "windowDuration") // 1 min – 2 hrs

		start := time.Unix(baseEpoch+startOffset, 0).UTC()
		stop := start.Add(time.Duration(windowDuration) * time.Second)

		type entry struct {
			timestamp time.Time
			inside    bool
		}

		var entries []entry

		// Generate 0–5 commands inside the window [start, stop].
		nInside := rapid.IntRange(0, 5).Draw(t, "nInside")
		for i := 0; i < nInside; i++ {
			offset := rapid.Int64Range(0, windowDuration).Draw(t, fmt.Sprintf("insideOffset%d", i))
			ts := start.Add(time.Duration(offset) * time.Second)
			entries = append(entries, entry{timestamp: ts, inside: true})
		}

		// Generate 1–5 commands strictly before start.
		nBefore := rapid.IntRange(1, 5).Draw(t, "nBefore")
		for i := 0; i < nBefore; i++ {
			offset := rapid.Int64Range(1, 3600).Draw(t, fmt.Sprintf("beforeOffset%d", i))
			ts := start.Add(-time.Duration(offset) * time.Second)
			entries = append(entries, entry{timestamp: ts, inside: false})
		}

		// Generate 1–5 commands strictly after stop.
		nAfter := rapid.IntRange(1, 5).Draw(t, "nAfter")
		for i := 0; i < nAfter; i++ {
			offset := rapid.Int64Range(1, 3600).Draw(t, fmt.Sprintf("afterOffset%d", i))
			ts := stop.Add(time.Duration(offset) * time.Second)
			entries = append(entries, entry{timestamp: ts, inside: false})
		}

		// Build a zsh extended-format history string.
		// Each command is uniquely identified by its index to avoid collisions.
		// Format: `: <epoch>:<elapsed>;<command>`
		var sb strings.Builder
		for i, e := range entries {
			fmt.Fprintf(&sb, ": %d:0;cmd%d\n", e.timestamp.Unix(), i)
		}

		// Parse with parseZshHistory.
		reader := strings.NewReader(sb.String())
		parsed, err := parseZshHistory(reader, start)
		if err != nil {
			t.Fatalf("parseZshHistory returned unexpected error: %v", err)
		}

		// Apply filterCommands with the session window.
		var warnings []string
		filtered := filterCommands(parsed, start, &stop, 0, &warnings)

		// Build a set of filtered command texts for O(1) lookup.
		filteredSet := make(map[string]bool, len(filtered))
		for _, c := range filtered {
			filteredSet[c.Raw] = true
		}

		// Assert every in-window command is present.
		for i, e := range entries {
			cmdName := fmt.Sprintf("cmd%d", i)
			if e.inside && !filteredSet[cmdName] {
				t.Fatalf("in-window command %q (ts=%v) missing from filtered result", cmdName, e.timestamp)
			}
		}

		// Assert no out-of-window command is present.
		for i, e := range entries {
			cmdName := fmt.Sprintf("cmd%d", i)
			if !e.inside && filteredSet[cmdName] {
				t.Fatalf("out-of-window command %q (ts=%v) unexpectedly present in filtered result", cmdName, e.timestamp)
			}
		}

		// Assert the total count matches the number of in-window entries.
		if len(filtered) != nInside {
			t.Fatalf("expected %d filtered commands, got %d", nInside, len(filtered))
		}
	})
}

// Feature: handoff, Property 5: Per-shell history parsing
func TestPerShellHistoryParsing(t *testing.T) {
	// --- Bash extended format ---
	t.Run("bash extended", func(t *testing.T) {
		rapid.Check(t, func(t *rapid.T) {
			n := rapid.IntRange(1, 10).Draw(t, "n")

			type entry struct {
				cmd   string
				epoch int64
			}
			
			entries := make([]entry, n)
			for i := 0; i < n; i++ {
				entries[i] = entry{
					cmd:   rapid.StringMatching(`[a-z][a-z0-9 ]{0,20}`).Draw(t, fmt.Sprintf("cmd%d", i)),
					epoch: rapid.Int64Range(1_000_000_000, 1_700_000_000).Draw(t, fmt.Sprintf("epoch%d", i)),
				}
			}

			// Build bash extended history string: #<epoch>\n<command>
			var sb strings.Builder
			for _, e := range entries {
				fmt.Fprintf(&sb, "#%d\n%s\n", e.epoch, e.cmd)
			}

			parsed, err := parseBashHistory(strings.NewReader(sb.String()), time.Time{})
			if err != nil {
				t.Fatalf("parseBashHistory returned unexpected error: %v", err)
			}
			if len(parsed) != n {
				t.Fatalf("expected %d commands, got %d", n, len(parsed))
			}
			for i, e := range entries {
				if parsed[i].Raw != e.cmd {
					t.Fatalf("entry %d: expected command %q, got %q", i, e.cmd, parsed[i].Raw)
				}
				if parsed[i].Timestamp.Unix() != e.epoch {
					t.Fatalf("entry %d: expected timestamp %d, got %d", i, e.epoch, parsed[i].Timestamp.Unix())
				}
			}
		})
	})

	// --- Zsh extended format ---
	t.Run("zsh extended", func(t *testing.T) {
		rapid.Check(t, func(t *rapid.T) {
			n := rapid.IntRange(1, 10).Draw(t, "n")

			type entry struct {
				cmd   string
				epoch int64
			}
			entries := make([]entry, n)
			for i := 0; i < n; i++ {
				entries[i] = entry{
					cmd:   rapid.StringMatching(`[a-z][a-z0-9 ]{0,20}`).Draw(t, fmt.Sprintf("cmd%d", i)),
					epoch: rapid.Int64Range(1_000_000_000, 1_700_000_000).Draw(t, fmt.Sprintf("epoch%d", i)),
				}
			}

			// Build zsh extended history string: : <epoch>:<elapsed>;<command>
			var sb strings.Builder
			for _, e := range entries {
				fmt.Fprintf(&sb, ": %d:0;%s\n", e.epoch, e.cmd)
			}

			parsed, err := parseZshHistory(strings.NewReader(sb.String()), time.Time{})
			if err != nil {
				t.Fatalf("parseZshHistory returned unexpected error: %v", err)
			}
			if len(parsed) != n {
				t.Fatalf("expected %d commands, got %d", n, len(parsed))
			}
			for i, e := range entries {
				if parsed[i].Raw != e.cmd {
					t.Fatalf("entry %d: expected command %q, got %q", i, e.cmd, parsed[i].Raw)
				}
				if parsed[i].Timestamp.Unix() != e.epoch {
					t.Fatalf("entry %d: expected timestamp %d, got %d", i, e.epoch, parsed[i].Timestamp.Unix())
				}
			}
		})
	})

	// --- Fish format ---
	t.Run("fish", func(t *testing.T) {
		rapid.Check(t, func(t *rapid.T) {
			n := rapid.IntRange(1, 10).Draw(t, "n")

			type entry struct {
				cmd   string
				epoch int64
			}
			entries := make([]entry, n)
			for i := 0; i < n; i++ {
				entries[i] = entry{
					cmd:   rapid.StringMatching(`[a-z][a-z0-9 ]{0,20}`).Draw(t, fmt.Sprintf("cmd%d", i)),
					epoch: rapid.Int64Range(1_000_000_000, 1_700_000_000).Draw(t, fmt.Sprintf("epoch%d", i)),
				}
			}

			// Build fish history string: - cmd: <command>\n  when: <epoch>
			var sb strings.Builder
			for _, e := range entries {
				fmt.Fprintf(&sb, "- cmd: %s\n  when: %d\n", e.cmd, e.epoch)
			}

			parsed, err := parseFishHistory(strings.NewReader(sb.String()), time.Time{})
			if err != nil {
				t.Fatalf("parseFishHistory returned unexpected error: %v", err)
			}
			if len(parsed) != n {
				t.Fatalf("expected %d commands, got %d", n, len(parsed))
			}
			for i, e := range entries {
				if parsed[i].Raw != e.cmd {
					t.Fatalf("entry %d: expected command %q, got %q", i, e.cmd, parsed[i].Raw)
				}
				if parsed[i].Timestamp.Unix() != e.epoch {
					t.Fatalf("entry %d: expected timestamp %d, got %d", i, e.epoch, parsed[i].Timestamp.Unix())
				}
			}
		})
	})
}

// TestShellDetection tests that the correct parser is selected based on the SHELL env var,
// verified indirectly by writing known history content and asserting correct parsing.
func TestShellDetection(t *testing.T) {
	// Session window: started 1 hour ago, stops 1 hour from now — all commands pass the filter.
	now := time.Now()
	sess := &session.Session{
		StartTime: now.Add(-1 * time.Hour),
		StopTime:  &[]time.Time{now.Add(1 * time.Hour)}[0],
	}

	t.Run("SHELL=/bin/bash selects bash parser", func(t *testing.T) {
		t.Setenv("SHELL", "/bin/bash")

		// Write a bash extended-format history file.
		f, err := os.CreateTemp(t.TempDir(), "bash_history")
		if err != nil {
			t.Fatalf("failed to create temp file: %v", err)
		}
		epoch := now.Unix()
		fmt.Fprintf(f, "#%d\nhello-bash\n", epoch)
		f.Close()

		sc := &ShellCollector{HistoryPath: f.Name()}
		result, err := sc.Collect(context.Background(), sess)
		if err != nil {
			t.Fatalf("Collect returned error: %v", err)
		}
		if len(result.Commands) != 1 {
			t.Fatalf("expected 1 command, got %d", len(result.Commands))
		}
		if result.Commands[0].Raw != "hello-bash" {
			t.Errorf("expected command %q, got %q", "hello-bash", result.Commands[0].Raw)
		}
	})

	t.Run("SHELL=/usr/bin/zsh selects zsh parser", func(t *testing.T) {
		t.Setenv("SHELL", "/usr/bin/zsh")

		f, err := os.CreateTemp(t.TempDir(), "zsh_history")
		if err != nil {
			t.Fatalf("failed to create temp file: %v", err)
		}
		epoch := now.Unix()
		fmt.Fprintf(f, ": %d:0;hello-zsh\n", epoch)
		f.Close()

		sc := &ShellCollector{HistoryPath: f.Name()}
		result, err := sc.Collect(context.Background(), sess)
		if err != nil {
			t.Fatalf("Collect returned error: %v", err)
		}
		if len(result.Commands) != 1 {
			t.Fatalf("expected 1 command, got %d", len(result.Commands))
		}
		if result.Commands[0].Raw != "hello-zsh" {
			t.Errorf("expected command %q, got %q", "hello-zsh", result.Commands[0].Raw)
		}
	})

	t.Run("SHELL=fish selects fish parser", func(t *testing.T) {
		t.Setenv("SHELL", "fish")

		f, err := os.CreateTemp(t.TempDir(), "fish_history")
		if err != nil {
			t.Fatalf("failed to create temp file: %v", err)
		}
		epoch := now.Unix()
		fmt.Fprintf(f, "- cmd: hello-fish\n  when: %d\n", epoch)
		f.Close()

		sc := &ShellCollector{HistoryPath: f.Name()}
		result, err := sc.Collect(context.Background(), sess)
		if err != nil {
			t.Fatalf("Collect returned error: %v", err)
		}
		if len(result.Commands) != 1 {
			t.Fatalf("expected 1 command, got %d", len(result.Commands))
		}
		if result.Commands[0].Raw != "hello-fish" {
			t.Errorf("expected command %q, got %q", "hello-fish", result.Commands[0].Raw)
		}
	})
}

// TestMissingHistoryFile tests that a missing history file produces a warning and empty commands.
func TestMissingHistoryFile(t *testing.T) {
	now := time.Now()
	sess := &session.Session{
		StartTime: now.Add(-1 * time.Hour),
		StopTime:  &[]time.Time{now.Add(1 * time.Hour)}[0],
	}

	sc := &ShellCollector{HistoryPath: "/nonexistent/path/to/history_file_xyz"}
	result, err := sc.Collect(context.Background(), sess)
	if err != nil {
		t.Fatalf("Collect returned unexpected error: %v", err)
	}
	if len(result.Commands) != 0 {
		t.Errorf("expected empty commands, got %d", len(result.Commands))
	}
	if len(result.Warnings) == 0 {
		t.Fatal("expected at least one warning for missing history file, got none")
	}
	// Verify the warning mentions the missing path.
	found := false
	for _, w := range result.Warnings {
		if strings.Contains(w, "history") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected warning to mention 'history', got: %v", result.Warnings)
	}
}
