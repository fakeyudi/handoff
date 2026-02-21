package collector

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/fakeyudi/handoff/internal/bundle"
	"github.com/fakeyudi/handoff/internal/session"
	shellpkg "github.com/fakeyudi/handoff/internal/shell"
)

// HistoryParser parses shell history from r, returning commands with timestamps >= since.
type HistoryParser func(r io.Reader, since time.Time) ([]bundle.Command, error)

// ShellCollector collects terminal command history from the user's shell.
type ShellCollector struct {
	// HistoryPath overrides the auto-detected history file path. If empty,
	// the path is derived from the SHELL environment variable.
	HistoryPath string
	// UsePluginLog instructs the collector to read from the handoff command log
	// (written by the shell plugin) instead of the shell history file.
	UsePluginLog bool
}

// Collect reads shell commands for the session window.
// If UsePluginLog is true and the log has entries, it uses those (accurate
// timestamps, no buffering issues). Otherwise falls back to the history file.
func (sc *ShellCollector) Collect(ctx context.Context, sess *session.Session) (CollectorResult, error) {
	if sc.UsePluginLog {
		cmds, err := shellpkg.ReadCommandLog()
		if err == nil && len(cmds) > 0 {
			// Filter to session window and strip noise.
			var warnings []string
			filtered := filterCommands(cmds, sess.StartTime, sess.StopTime, 0, &warnings)
			// Truncate the log now that we've consumed it.
			_ = shellpkg.TruncateCommandLog()
			return CollectorResult{Commands: filtered, Warnings: warnings}, nil
		}
		// Log empty or unreadable — fall through to history file with a hint.
	}

	return sc.collectFromHistory(sess)
}

// collectFromHistory reads the shell history file as a fallback.
func (sc *ShellCollector) collectFromHistory(sess *session.Session) (CollectorResult, error) {
	shell := filepath.Base(os.Getenv("SHELL"))

	var parser HistoryParser
	var defaultPath string
	switch shell {
	case "bash":
		parser = parseBashHistory
		home, _ := os.UserHomeDir()
		defaultPath = filepath.Join(home, ".bash_history")
	case "zsh":
		parser = parseZshHistory
		home, _ := os.UserHomeDir()
		defaultPath = filepath.Join(home, ".zsh_history")
	case "fish":
		parser = parseFishHistory
		home, _ := os.UserHomeDir()
		defaultPath = filepath.Join(home, ".local", "share", "fish", "fish_history")
	default:
		// Unknown shell — try bash as a best-effort fallback.
		parser = parseBashHistory
		home, _ := os.UserHomeDir()
		defaultPath = filepath.Join(home, ".bash_history")
	}

	histPath := sc.HistoryPath
	if histPath == "" {
		histPath = defaultPath
	}

	f, err := os.Open(histPath)
	if err != nil {
		warning := fmt.Sprintf("shell history unavailable (%s): %v", histPath, err)
		return CollectorResult{Warnings: []string{warning}}, nil
	}
	defer f.Close()

	commands, err := parser(f, sess.StartTime)
	if err != nil {
		warning := fmt.Sprintf("failed to parse shell history (%s): %v", histPath, err)
		return CollectorResult{Warnings: []string{warning}}, nil
	}

	// Filter to [StartTime, StopTime] if StopTime is set.
	var warnings []string
	filtered := filterCommands(commands, sess.StartTime, sess.StopTime, sess.HistoryBaselineCount, &warnings)

	return CollectorResult{
		Commands: filtered,
		Warnings: warnings,
	}, nil
}

// maxNoTimestampCommands is the maximum number of recent commands to include
// when the shell history has no timestamps.
const maxNoTimestampCommands = 50

// handoffNoiseCommands are handoff subcommands that are pure session
// bookkeeping and should never appear in the commands list.
var handoffNoiseCommands = []string{"start", "stop"}

// isHandoffNoise returns true if the command is a handoff start/stop invocation.
func isHandoffNoise(raw string) bool {
	// Match any invocation of the binary followed by start or stop.
	// e.g. "handoff start", "./handoff stop", "/usr/local/bin/handoff start"
	fields := strings.Fields(raw)
	if len(fields) < 2 {
		return false
	}
	bin := filepath.Base(fields[0])
	if bin != "handoff" {
		return false
	}
	for _, noise := range handoffNoiseCommands {
		if fields[1] == noise {
			return true
		}
	}
	return false
}

// filterCommands filters commands to the session window [start, stop].
// For no-timestamp shells it skips the first baselineCount entries (commands
// that existed before the session started) and then takes up to
// maxNoTimestampCommands of what remains.
// handoff start/stop commands are always stripped.
func filterCommands(commands []bundle.Command, start time.Time, stop *time.Time, baselineCount int, warnings *[]string) []bundle.Command {
	var timestamped, noTimestamp []bundle.Command

	for _, cmd := range commands {
		if isHandoffNoise(cmd.Raw) {
			continue
		}
		if cmd.Timestamp.IsZero() {
			noTimestamp = append(noTimestamp, cmd)
		} else {
			timestamped = append(timestamped, cmd)
		}
	}

	var result []bundle.Command

	// Filter timestamped commands to the session window.
	for _, cmd := range timestamped {
		if cmd.Timestamp.Before(start) {
			continue
		}
		if stop != nil && cmd.Timestamp.After(*stop) {
			continue
		}
		result = append(result, cmd)
	}

	// For no-timestamp commands: skip the baseline prefix, then take the tail.
	if len(noTimestamp) > 0 {
		fresh := noTimestamp
		if baselineCount > 0 && baselineCount < len(fresh) {
			fresh = fresh[baselineCount:]
		} else if baselineCount >= len(fresh) {
			// History count at stop == baseline: shell hasn't flushed new commands yet.
			// This happens when INC_APPEND_HISTORY / HISTFILE sharing is not enabled.
			*warnings = append(*warnings, "shell history was not flushed during session — "+
				"add 'setopt INC_APPEND_HISTORY' to ~/.zshrc (zsh) or "+
				"'PROMPT_COMMAND=\"history -a\"' to ~/.bashrc (bash) to capture commands in real time")
			fresh = nil
		}
		if len(fresh) > maxNoTimestampCommands {
			fresh = fresh[len(fresh)-maxNoTimestampCommands:]
		}
		if len(fresh) > 0 {
			*warnings = append(*warnings, fmt.Sprintf("shell history has no timestamps; showing %d commands since session start", len(fresh)))
			result = append(result, fresh...)
		}
	}

	return result
}

// SnapshotHistoryBaseline returns the total number of commands currently in
// the shell history file. Stored at session start so the stop collector can
// skip that many entries and only show commands typed during the session.
func SnapshotHistoryBaseline(historyPathOverride string) int {
	shell := filepath.Base(os.Getenv("SHELL"))

	var parser HistoryParser
	var defaultPath string
	home, _ := os.UserHomeDir()

	switch shell {
	case "bash":
		parser = parseBashHistory
		defaultPath = filepath.Join(home, ".bash_history")
	case "zsh":
		parser = parseZshHistory
		defaultPath = filepath.Join(home, ".zsh_history")
	case "fish":
		parser = parseFishHistory
		defaultPath = filepath.Join(home, ".local", "share", "fish", "fish_history")
	default:
		parser = parseBashHistory
		defaultPath = filepath.Join(home, ".bash_history")
	}

	histPath := historyPathOverride
	if histPath == "" {
		histPath = defaultPath
	}

	f, err := os.Open(histPath)
	if err != nil {
		return 0
	}
	defer f.Close()

	commands, err := parser(f, time.Time{})
	if err != nil {
		return 0
	}
	return len(commands)
}

// flushShellHistory attempts to flush the current shell's in-memory history to
// disk before we read the history file. This is necessary for zsh (and bash
// with HISTFILE) because history is only written on shell exit by default.
// We run the flush in a new shell subprocess so it affects the history file
// even when called from a non-interactive context.
func flushShellHistory(shell string) {
	var cmd *exec.Cmd
	switch shell {
	case "zsh":
		// fc -W writes the in-memory history of the current session to HISTFILE.
		// Running it in a login shell ensures HISTFILE is set correctly.
		cmd = exec.Command("zsh", "-i", "-c", "fc -W")
	case "bash":
		cmd = exec.Command("bash", "-i", "-c", "history -w")
	default:
		return
	}
	// Ignore errors — this is best-effort.
	_ = cmd.Run()
}
//
// Format:
//   - Plain: one command per line (no timestamps).
//   - With HISTTIMEFORMAT: a `#<epoch>` line precedes each command.
func parseBashHistory(r io.Reader, since time.Time) ([]bundle.Command, error) {
	var commands []bundle.Command
	scanner := bufio.NewScanner(r)

	var pendingTime time.Time

	for scanner.Scan() {
		line := scanner.Text()

		if strings.HasPrefix(line, "#") {
			// Possible timestamp line: #<epoch>
			epochStr := strings.TrimPrefix(line, "#")
			if epoch, err := strconv.ParseInt(epochStr, 10, 64); err == nil {
				pendingTime = time.Unix(epoch, 0)
				continue
			}
			// Otherwise it's a comment — skip.
			pendingTime = time.Time{}
			continue
		}

		if line == "" {
			pendingTime = time.Time{}
			continue
		}

		commands = append(commands, bundle.Command{
			Raw:       line,
			Timestamp: pendingTime,
		})
		pendingTime = time.Time{}
	}

	return commands, scanner.Err()
}

// parseZshHistory parses ~/.zsh_history.
//
// Extended format: `: <epoch>:<elapsed>;<command>`
// Plain fallback:  one command per line.
func parseZshHistory(r io.Reader, since time.Time) ([]bundle.Command, error) {
	var commands []bundle.Command
	scanner := bufio.NewScanner(r)

	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}

		// Try extended format: `: <epoch>:<elapsed>;<command>`
		if strings.HasPrefix(line, ": ") {
			// Strip the leading ": "
			rest := line[2:]
			// Find the semicolon separating elapsed from command
			semiIdx := strings.Index(rest, ";")
			if semiIdx > 0 {
				timePart := rest[:semiIdx]
				cmd := rest[semiIdx+1:]

				// timePart is "<epoch>:<elapsed>"
				colonIdx := strings.Index(timePart, ":")
				if colonIdx > 0 {
					epochStr := timePart[:colonIdx]
					if epoch, err := strconv.ParseInt(epochStr, 10, 64); err == nil {
						commands = append(commands, bundle.Command{
							Raw:       cmd,
							Timestamp: time.Unix(epoch, 0),
						})
						continue
					}
				}
			}
		}

		// Plain fallback — no timestamp.
		commands = append(commands, bundle.Command{
			Raw: line,
		})
	}

	return commands, scanner.Err()
}

// parseFishHistory parses ~/.local/share/fish/fish_history.
//
// YAML-like format:
//
//	- cmd: <command>
//	  when: <epoch>
func parseFishHistory(r io.Reader, since time.Time) ([]bundle.Command, error) {
	var commands []bundle.Command
	scanner := bufio.NewScanner(r)

	var currentCmd string
	var currentTime time.Time
	inEntry := false

	for scanner.Scan() {
		line := scanner.Text()

		if strings.HasPrefix(line, "- cmd: ") {
			// Save previous entry if any.
			if inEntry && currentCmd != "" {
				commands = append(commands, bundle.Command{
					Raw:       currentCmd,
					Timestamp: currentTime,
				})
			}
			currentCmd = strings.TrimPrefix(line, "- cmd: ")
			currentTime = time.Time{}
			inEntry = true
			continue
		}

		if inEntry && strings.HasPrefix(line, "  when: ") {
			epochStr := strings.TrimPrefix(line, "  when: ")
			if epoch, err := strconv.ParseInt(strings.TrimSpace(epochStr), 10, 64); err == nil {
				currentTime = time.Unix(epoch, 0)
			}
			continue
		}

		// Any other line (e.g. "  paths:") — just skip, stay in entry.
	}

	// Flush the last entry.
	if inEntry && currentCmd != "" {
		commands = append(commands, bundle.Command{
			Raw:       currentCmd,
			Timestamp: currentTime,
		})
	}

	return commands, scanner.Err()
}
