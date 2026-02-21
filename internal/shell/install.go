// Package shell handles shell plugin installation and command log reading.
package shell

import (
	"fmt"
	"os"
	"path/filepath"
)

// PluginPath returns the path where the plugin file should be written.
func PluginPath(shell string) (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	name := "handoff.plugin." + shell
	return filepath.Join(home, ".config", "handoff", name), nil
}

// Install writes the plugin file for the given shell and prints the source
// instruction the user needs to add to their rc file.
func Install(shell string) error {
	path, err := PluginPath(shell)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}

	var content string
	switch shell {
	case "zsh":
		content = ZshPlugin
	case "bash":
		content = BashPlugin
	default:
		return fmt.Errorf("unsupported shell for plugin: %s (supported: zsh, bash)", shell)
	}

	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return fmt.Errorf("writing plugin file: %w", err)
	}

	rcFile := rcFileName(shell)
	fmt.Printf("\n  ✓ Plugin written to %s\n", path)
	fmt.Printf("\n  Add this line to your %s:\n", rcFile)
	fmt.Printf("    source %s\n", path)
	fmt.Printf("\n  Then reload: source %s\n\n", rcFile)
	return nil
}

// IsInstalled reports whether the plugin file exists on disk.
func IsInstalled(shell string) bool {
	path, err := PluginPath(shell)
	if err != nil {
		return false
	}
	_, err = os.Stat(path)
	return err == nil
}

func rcFileName(shell string) string {
	switch shell {
	case "zsh":
		return "~/.zshrc"
	case "bash":
		return "~/.bashrc"
	default:
		return "~/." + shell + "rc"
	}
}
