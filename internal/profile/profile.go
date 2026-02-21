// Package profile manages the user's persistent handoff profile.
// The profile is stored at ~/.config/handoff/profile.json and is created
// once via the interactive setup flow, then referenced on every command.
package profile

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Profile holds user-level preferences set during first-run setup.
type Profile struct {
	Name              string `json:"name"`
	DefaultFormat     string `json:"default_format"`      // "markdown" | "json"
	RecordCommands    bool   `json:"record_commands"`     // install shell plugin
	OutputDir         string `json:"output_dir"`          // default bundle output dir
	ShellPluginShell  string `json:"shell_plugin_shell"`  // "zsh" | "bash" | ""
}

// profilePath returns the path to the profile file.
func profilePath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config", "handoff", "profile.json"), nil
}

// ConfigDir returns the handoff config directory.
func ConfigDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config", "handoff"), nil
}

// Exists reports whether a profile file is present on disk.
func Exists() bool {
	p, err := profilePath()
	if err != nil {
		return false
	}
	_, err = os.Stat(p)
	return err == nil
}

// Load reads the profile from disk. Returns an error if the file is missing or malformed.
func Load() (*Profile, error) {
	p, err := profilePath()
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(p)
	if err != nil {
		return nil, fmt.Errorf("profile not found — run 'handoff setup' to configure: %w", err)
	}
	var prof Profile
	if err := json.Unmarshal(data, &prof); err != nil {
		return nil, fmt.Errorf("malformed profile at %s: %w", p, err)
	}
	return &prof, nil
}

// Save writes the profile to disk, creating the config directory if needed.
func Save(prof *Profile) error {
	p, err := profilePath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(prof, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(p, data, 0o644)
}

// RunSetup runs the interactive setup wizard and saves the resulting profile.
// If existing is non-nil, it is used as the default for each prompt (edit mode).
func RunSetup(existing *Profile) (*Profile, error) {
	r := bufio.NewReader(os.Stdin)

	ask := func(prompt, defaultVal string) (string, error) {
		if defaultVal != "" {
			fmt.Printf("%s [%s]: ", prompt, defaultVal)
		} else {
			fmt.Printf("%s: ", prompt)
		}
		line, err := r.ReadString('\n')
		if err != nil {
			return "", err
		}
		line = strings.TrimSpace(line)
		if line == "" {
			return defaultVal, nil
		}
		return line, nil
	}

	askBool := func(prompt string, defaultVal bool) (bool, error) {
		def := "n"
		if defaultVal {
			def = "y"
		}
		ans, err := ask(prompt+" (y/n)", def)
		if err != nil {
			return false, err
		}
		return strings.ToLower(ans) == "y" || strings.ToLower(ans) == "yes", nil
	}

	prof := &Profile{
		DefaultFormat:  "markdown",
		OutputDir:      ".",
		RecordCommands: true,
	}
	if existing != nil {
		*prof = *existing
	}

	fmt.Println()
	fmt.Println("  ┌─────────────────────────────────┐")
	fmt.Println("  │   handoff — first-time setup    │")
	fmt.Println("  └─────────────────────────────────┘")
	fmt.Println()

	var err error

	prof.Name, err = ask("  Your name (shown in bundles)", prof.Name)
	if err != nil {
		return nil, err
	}

	format, err := ask("  Default output format (markdown/json)", prof.DefaultFormat)
	if err != nil {
		return nil, err
	}
	if format == "json" {
		prof.DefaultFormat = "json"
	} else {
		prof.DefaultFormat = "markdown"
	}

	prof.OutputDir, err = ask("  Default output directory", prof.OutputDir)
	if err != nil {
		return nil, err
	}

	prof.RecordCommands, err = askBool("  Record terminal commands via shell plugin", prof.RecordCommands)
	if err != nil {
		return nil, err
	}

	if prof.RecordCommands {
		shell, err := ask("  Shell (zsh/bash)", detectShell())
		if err != nil {
			return nil, err
		}
		prof.ShellPluginShell = shell
	} else {
		prof.ShellPluginShell = ""
	}

	fmt.Println()
	return prof, nil
}

// detectShell returns the base name of the current shell.
func detectShell() string {
	shell := filepath.Base(os.Getenv("SHELL"))
	if shell == "zsh" || shell == "bash" {
		return shell
	}
	return "zsh"
}
