package collector

import (
	"context"
	"errors"
	"os/exec"
	"strings"
	"time"

	"github.com/fakeyudi/handoff/internal/bundle"
	"github.com/fakeyudi/handoff/internal/session"
)

// GitRunner executes a git command and returns its output.
// This abstraction allows mocking in tests.
type GitRunner func(workDir string, args ...string) (string, error)

// GitCollector collects git repository state.
type GitCollector struct {
	WorkDir string
	Runner  GitRunner // if nil, uses the real git subprocess
}

// defaultGitRunner runs git as a real subprocess.
func defaultGitRunner(workDir string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = workDir
	out, err := cmd.Output()
	return string(out), err
}

// Collect implements Collector. It runs several git commands to capture
// the current repository state and populates a GitInfo in the result.
// If the working directory is not a git repository (exit code 128), it
// appends a warning and returns a result with nil GitInfo.
func (g *GitCollector) Collect(ctx context.Context, sess *session.Session) (CollectorResult, error) {
	runner := g.Runner
	if runner == nil {
		runner = defaultGitRunner
	}

	workDir := g.WorkDir
	if workDir == "" {
		workDir = sess.WorkDir
	}

	// Determine branch — also serves as the "is this a git repo?" check.
	branch, err := runner(workDir, "rev-parse", "--abbrev-ref", "HEAD")
	if err != nil {
		if isExitCode128(err) {
			return CollectorResult{
				Warnings: []string{"not a git repository"},
			}, nil
		}
		return CollectorResult{}, err
	}

	headCommit, err := runner(workDir, "rev-parse", "HEAD")
	if err != nil {
		return CollectorResult{}, err
	}

	diff, err := runner(workDir, "diff")
	if err != nil {
		return CollectorResult{}, err
	}

	stagedDiff, err := runner(workDir, "diff", "--staged")
	if err != nil {
		return CollectorResult{}, err
	}

	since := sess.StartTime.Format(time.RFC3339)
	logOut, err := runner(workDir, "log", "--oneline", "--since="+since)
	if err != nil {
		return CollectorResult{}, err
	}

	recentLog := parseLogLines(logOut)

	info := &bundle.GitInfo{
		Branch:     strings.TrimSpace(branch),
		HeadCommit: strings.TrimSpace(headCommit),
		Diff:       diff,
		StagedDiff: stagedDiff,
		RecentLog:  recentLog,
	}

	return CollectorResult{GitInfo: info}, nil
}

// isExitCode128 reports whether err is an *exec.ExitError with exit code 128.
func isExitCode128(err error) bool {
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return exitErr.ExitCode() == 128
	}
	return false
}

// parseLogLines splits git log output into individual commit lines,
// discarding empty lines.
func parseLogLines(output string) []string {
	lines := strings.Split(output, "\n")
	result := make([]string, 0, len(lines))
	for _, l := range lines {
		if l != "" {
			result = append(result, l)
		}
	}
	return result
}
