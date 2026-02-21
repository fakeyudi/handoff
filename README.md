# Handoff

A CLI tool that tracks your developer activity during a session and generates a shareable "context bundle" — so a teammate can pick up exactly where you left off.

It captures file edits, terminal commands, git diffs, open editor tabs, and your own notes, then packages everything into a readable Markdown (or JSON) document. Try it out yourself

## Install

### Homebrew

```bash
# Add the tap (if not in Homebrew Core)
brew tap fakeyudi/handoff

# Install
brew install handoff
```

### From Source

```bash
git clone <repo>
cd handoff
go install .
```

Or build a binary directly:

```bash
go build -o handoff .
```

### Prebuilt Binaries
Download from [Releases](https://github.com/fakeyudi/handoff/releases).

## Quick Start

```bash
# 1. Start a session in your project directory (For local builds you might need to provide path like ./handoff)
handoff start

# 2. Work normally — edit files, run commands, etc.

# 3. Add notes as you go
handoff note "tried increasing the timeout, still flaky"
handoff note "suspect the issue is in the retry logic around line 42"

# 4. Check session status at any time
handoff status

# 5. Stop and generate the bundle
handoff stop -m "leaving off here, retry loop needs a closer look"
```

This produces a file like `handoff-2026-02-19T17:30:00Z.md` in the current directory.

## Commands

### `handoff start`

Begins a new tracking session. Records the start time and working directory. Persists state to disk so it survives terminal restarts.

```bash
handoff start
```

Errors if a session is already active.

### `handoff stop`

Ends the session, runs all collectors, and writes the context bundle.

```bash
handoff stop
handoff stop -m "summary of where things stand"
handoff stop --format json
```

Flags:
- `-m, --message` — adds a summary annotation to the bundle
- `--format` — `markdown` (default) or `json`

### `handoff note`

Appends a timestamped annotation to the active session.

```bash
handoff note "reproduced the bug with payload > 1MB"
```

Errors if no session is active.

### `handoff status`

Shows the current session state.

```bash
handoff status
```

Output includes start time, elapsed duration, number of file edits tracked, and number of annotations recorded.

### `handoff view`

Parses and displays a context bundle file.

```bash
handoff view handoff-2026-02-19T17:30:00Z.md
handoff view handoff-2026-02-19T17:30:00Z.json
```

Sections are displayed in order: Summary → Annotations → File Edits → Git Changes → Terminal Commands → Editor Tabs.

## Output Format

### Markdown (default)

Human-readable with fenced code blocks. Also embeds a base64 JSON payload in HTML comments at the top for lossless round-trip parsing:

```
<!-- handoff-bundle-version: 1 -->
<!-- handoff-data: <base64> -->

# Handoff — /your/project — 2026-02-19T17:30:00Z
...
```

### JSON

Full structured output, useful for programmatic consumption:

```bash
handoff stop --format json
```

## Configuration

Handoff merges two optional config files. Project-level settings take precedence over global.

| File | Scope |
|------|-------|
| `~/.config/handoff/config.json` | Global (all projects) |
| `.handoffconfig` | Project-level |

Both files use the same JSON format:

```json
{
  "ignore_patterns": ["*.log", "node_modules", ".git"],
  "shell_history_path": "/custom/path/to/history",
  "default_format": "markdown",
  "output_dir": "./handoffs"
}
```

| Key | Default | Description |
|-----|---------|-------------|
| `ignore_patterns` | `[]` | Glob patterns to exclude from file edit tracking. Also reads `.gitignore` and `.handoffignore` automatically. |
| `shell_history_path` | auto-detected | Override the shell history file path. |
| `default_format` | `"markdown"` | Default bundle format: `"markdown"` or `"json"`. |
| `output_dir` | `"."` | Directory where bundle files are written. |

## Shell Support

Handoff auto-detects your shell via the `SHELL` environment variable and reads the appropriate history file:

| Shell | History File |
|-------|-------------|
| bash | `~/.bash_history` |
| zsh | `~/.zsh_history` |
| fish | `~/.local/share/fish/fish_history` |

If the history file is missing or unreadable, a warning is printed to stderr and the bundle is generated without terminal history.



## Development

```bash
# Run tests
go test ./...

# Run with verbose output
go test ./... -v

# Run a specific package
go test ./internal/bundle/...
```
