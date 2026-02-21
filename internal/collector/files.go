package collector

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/fakeyudi/handoff/internal/session"
)

// FileCollector collects file edit events from the working directory.
type FileCollector struct {
	WorkDir        string
	IgnorePatterns []string
}

// Collect finds files modified within the session time window by walking the
// working directory and checking each file's mtime against sess.StartTime.
// It also merges any FileEdits already recorded in the session (from the
// background watcher, if running), deduplicating by keeping the latest timestamp.
func (fc *FileCollector) Collect(ctx context.Context, sess *session.Session) (CollectorResult, error) {
	patterns, err := fc.loadIgnorePatterns()
	if err != nil {
		// Non-fatal: log as warning and continue with configured patterns only.
		return CollectorResult{
			Warnings: []string{"failed to load ignore patterns: " + err.Error()},
		}, nil
	}

	stopTime := time.Now()
	if sess.StopTime != nil {
		stopTime = *sess.StopTime
	}

	// Seed with any edits already recorded in the session (background watcher).
	latest := make(map[string]time.Time, len(sess.FileEdits))
	for _, fe := range sess.FileEdits {
		if t, ok := latest[fe.Path]; !ok || fe.Timestamp.After(t) {
			latest[fe.Path] = fe.Timestamp
		}
	}

	// Walk the working directory and collect files whose mtime falls within
	// [sess.StartTime, stopTime].
	workDir := fc.WorkDir
	if workDir == "" {
		workDir = "."
	}
	_ = filepath.WalkDir(workDir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil // skip unreadable entries
		}
		if d.IsDir() {
			return nil
		}
		if fc.isIgnored(path, patterns) {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return nil
		}
		mtime := info.ModTime()
		if mtime.Before(sess.StartTime) || mtime.After(stopTime) {
			return nil
		}
		if t, ok := latest[path]; !ok || mtime.After(t) {
			latest[path] = mtime
		}
		return nil
	})

	// Build the result slice, applying ignore patterns.
	var edits []session.FileEdit
	for path, ts := range latest {
		if fc.isIgnored(path, patterns) {
			continue
		}
		diff := captureFileDiff(path, fc.WorkDir)
		edits = append(edits, session.FileEdit{Path: path, Timestamp: ts, Diff: diff})
	}

	return CollectorResult{FileEdits: edits}, nil
}

// Watch starts a recursive fsnotify watcher on workDir and records Write/Create
// events into the store until ctx is cancelled. This is called from `handoff start`.
func Watch(ctx context.Context, workDir string, store session.SessionStore, ignorePatterns []string) error {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return err
	}
	defer watcher.Close()

	// Walk the directory tree and add a watcher for every subdirectory.
	if err := filepath.WalkDir(workDir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil // skip unreadable entries
		}
		if d.IsDir() {
			return watcher.Add(path)
		}
		return nil
	}); err != nil {
		return err
	}

	// Load ignore patterns (gitignore + handoffignore + configured patterns).
	fc := &FileCollector{WorkDir: workDir, IgnorePatterns: ignorePatterns}
	patterns, _ := fc.loadIgnorePatterns()

	for {
		select {
		case <-ctx.Done():
			return nil

		case event, ok := <-watcher.Events:
			if !ok {
				return nil
			}
			if event.Has(fsnotify.Write) || event.Has(fsnotify.Create) {
				if fc.isIgnored(event.Name, patterns) {
					continue
				}
				// Load current session, append the edit, and save atomically.
				sess, err := store.Load()
				if err != nil {
					continue
				}
				sess.FileEdits = append(sess.FileEdits, session.FileEdit{
					Path:      event.Name,
					Timestamp: time.Now(),
				})
				_ = store.Save(sess) // best-effort; don't crash the watcher on save failure

				// If a new directory was created, watch it too.
				if event.Has(fsnotify.Create) {
					if info, err := os.Stat(event.Name); err == nil && info.IsDir() {
						_ = watcher.Add(event.Name)
					}
				}
			}

		case _, ok := <-watcher.Errors:
			if !ok {
				return nil
			}
			// Watcher errors are non-fatal; continue watching.
		}
	}
}

// isIgnored reports whether path matches any of the given glob patterns.
func (fc *FileCollector) isIgnored(path string, patterns []string) bool {
	// Normalise to a relative path for matching when possible.
	rel := path
	if fc.WorkDir != "" {
		if r, err := filepath.Rel(fc.WorkDir, path); err == nil {
			rel = r
		}
	}
	base := filepath.Base(path)

	for _, pattern := range patterns {
		// Match against the base name.
		if matched, _ := filepath.Match(pattern, base); matched {
			return true
		}
		// Match against the relative path.
		if matched, _ := filepath.Match(pattern, rel); matched {
			return true
		}
		// Match against the full path.
		if matched, _ := filepath.Match(pattern, path); matched {
			return true
		}
	}
	return false
}

// loadIgnorePatterns merges the configured patterns with those from .gitignore
// and .handoffignore files found in the working directory.
func (fc *FileCollector) loadIgnorePatterns() ([]string, error) {
	patterns := make([]string, len(fc.IgnorePatterns))
	copy(patterns, fc.IgnorePatterns)

	for _, name := range []string{".gitignore", ".handoffignore"} {
		p := filepath.Join(fc.WorkDir, name)
		extra, err := readPatternFile(p)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return patterns, err
		}
		patterns = append(patterns, extra...)
	}
	return patterns, nil
}

// readPatternFile reads a gitignore-style file and returns non-empty, non-comment lines.
func readPatternFile(path string) ([]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var patterns []string
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		patterns = append(patterns, line)
	}
	return patterns, scanner.Err()
}

// captureFileDiff returns a unified diff for the given file.
// It first tries git (diff HEAD, then --cached). If git is unavailable or the
// file is not tracked, it falls back to a pure-Go diff against an empty file,
// effectively showing the full file content as additions.
func captureFileDiff(path, workDir string) string {
	run := func(args ...string) string {
		cmd := exec.Command("git", args...)
		cmd.Dir = workDir
		var out bytes.Buffer
		cmd.Stdout = &out
		if err := cmd.Run(); err != nil {
			return ""
		}
		return strings.TrimRight(out.String(), "\n")
	}

	if d := run("diff", "HEAD", "--", path); d != "" {
		return d
	}
	if d := run("diff", "--cached", "--", path); d != "" {
		return d
	}

	// Fallback: show the file as a pure addition diff.
	return fallbackDiff(path)
}

// fallbackDiff produces a simple unified diff showing the full file as added lines.
// Used when git is unavailable or the file is untracked.
func fallbackDiff(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	lines := strings.Split(strings.TrimRight(string(data), "\n"), "\n")
	var sb strings.Builder
	fmt.Fprintf(&sb, "--- /dev/null\n+++ %s\n@@ -0,0 +1,%d @@\n", path, len(lines))
	for _, l := range lines {
		sb.WriteString("+" + l + "\n")
	}
	return strings.TrimRight(sb.String(), "\n")
}
