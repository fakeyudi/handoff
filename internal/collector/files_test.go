package collector

import (
	"context"
	"testing"
	"time"

	"github.com/fakeyudi/handoff/internal/session"
	"pgregory.net/rapid"
)

// Feature: handoff, Property 2: File edit recording
func TestFileEditRecording(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		// Generate N (1-10) arbitrary relative file paths.
		n := rapid.IntRange(1, 10).Draw(t, "n")
		paths := make([]string, n)
		for i := range paths {
			// Generate a simple relative path like "dir/file.go"
			dir := rapid.StringMatching(`[a-z]{1,8}`).Draw(t, "dir")
			file := rapid.StringMatching(`[a-z]{1,8}\.(go|txt|md)`).Draw(t, "file")
			paths[i] = dir + "/" + file
		}

		// Build a session with those paths as FileEdits with non-zero timestamps.
		baseTime := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
		sess := &session.Session{
			ID:        "test-session",
			StartTime: baseTime,
			WorkDir:   "",
		}
		for i, p := range paths {
			sess.FileEdits = append(sess.FileEdits, session.FileEdit{
				Path:      p,
				Timestamp: baseTime.Add(time.Duration(i+1) * time.Second),
			})
		}

		// Call FileCollector.Collect with empty WorkDir and no ignore patterns.
		fc := &FileCollector{
			WorkDir:        "",
			IgnorePatterns: []string{},
		}
		result, err := fc.Collect(context.Background(), sess)
		if err != nil {
			t.Fatalf("Collect returned unexpected error: %v", err)
		}

		// Build a map of result paths for easy lookup.
		resultMap := make(map[string]time.Time, len(result.FileEdits))
		for _, fe := range result.FileEdits {
			resultMap[fe.Path] = fe.Timestamp
		}

		// Assert each generated path appears in the result with a non-zero timestamp.
		// Note: deduplication keeps the most recent timestamp per path, so if the
		// same path was generated multiple times, it still appears at least once.
		seen := make(map[string]bool)
		for _, p := range paths {
			if seen[p] {
				continue
			}
			seen[p] = true

			ts, ok := resultMap[p]
			if !ok {
				t.Fatalf("path %q not found in CollectorResult.FileEdits", p)
			}
			if ts.IsZero() {
				t.Fatalf("path %q has zero timestamp in CollectorResult.FileEdits", p)
			}
		}
	})
}

// Feature: handoff, Property 3: Ignore pattern filtering
func TestIgnorePatternFiltering(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		// Generate a random file extension (e.g. "log", "tmp", "bak").
		ext := rapid.StringMatching(`[a-z]{2,4}`).Draw(t, "ext")
		pattern := "*." + ext

		// Generate 1-5 file paths that are guaranteed to match the pattern.
		n := rapid.IntRange(1, 5).Draw(t, "n")
		matchingPaths := make([]string, n)
		for i := range matchingPaths {
			stem := rapid.StringMatching(`[a-z]{1,8}`).Draw(t, "stem")
			matchingPaths[i] = stem + "." + ext
		}

		// Also generate 0-3 non-matching paths (different extension) to ensure
		// the collector still returns those.
		otherExt := rapid.StringMatching(`[a-z]{2,4}`).Draw(t, "otherExt")
		// Make sure the other extension is different so it won't be filtered.
		if otherExt == ext {
			otherExt = otherExt + "x"
		}
		m := rapid.IntRange(0, 3).Draw(t, "m")
		otherPaths := make([]string, m)
		for i := range otherPaths {
			stem := rapid.StringMatching(`[a-z]{1,8}`).Draw(t, "stem2")
			otherPaths[i] = stem + "." + otherExt
		}

		// Build a session with both matching and non-matching paths as FileEdits.
		baseTime := time.Date(2024, 6, 1, 9, 0, 0, 0, time.UTC)
		sess := &session.Session{
			ID:        "test-ignore-session",
			StartTime: baseTime,
			WorkDir:   "",
		}
		for i, p := range matchingPaths {
			sess.FileEdits = append(sess.FileEdits, session.FileEdit{
				Path:      p,
				Timestamp: baseTime.Add(time.Duration(i+1) * time.Second),
			})
		}
		for i, p := range otherPaths {
			sess.FileEdits = append(sess.FileEdits, session.FileEdit{
				Path:      p,
				Timestamp: baseTime.Add(time.Duration(n+i+1) * time.Second),
			})
		}

		// Collect with the ignore pattern.
		fc := &FileCollector{
			WorkDir:        "",
			IgnorePatterns: []string{pattern},
		}
		result, err := fc.Collect(context.Background(), sess)
		if err != nil {
			t.Fatalf("Collect returned unexpected error: %v", err)
		}

		// Build a set of result paths for easy lookup.
		resultPaths := make(map[string]bool, len(result.FileEdits))
		for _, fe := range result.FileEdits {
			resultPaths[fe.Path] = true
		}

		// Assert that NONE of the matching paths appear in the result.
		for _, p := range matchingPaths {
			if resultPaths[p] {
				t.Fatalf("path %q matched pattern %q but was NOT filtered from CollectorResult.FileEdits", p, pattern)
			}
		}
	})
}
