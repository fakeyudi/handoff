package config

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
)

// Config holds all configurable Handoff settings.
type Config struct {
	IgnorePatterns   []string `json:"ignore_patterns"`
	ShellHistoryPath string   `json:"shell_history_path"` // override auto-detect
	DefaultFormat    string   `json:"default_format"`     // "markdown" | "json"
	OutputDir        string   `json:"output_dir"`
}

// Defaults returns sensible default configuration values.
func Defaults() Config {
	return Config{
		DefaultFormat:  "markdown",
		OutputDir:      ".",
		IgnorePatterns: []string{},
	}
}

// LoadGlobal reads ~/.config/handoff/config.json.
// Returns defaults if the file is absent.
func LoadGlobal() (*Config, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	path := filepath.Join(home, ".config", "handoff", "config.json")
	return loadFile(path, true)
}

// LoadProject reads .handoffconfig in the current working directory.
// Returns nil (no error) if the file is absent.
func LoadProject() (*Config, error) {
	return loadFile(".handoffconfig", false)
}

// loadFile reads and parses a JSON config file at path.
// If returnDefaults is true, returns defaults when the file is absent.
// If returnDefaults is false, returns nil when the file is absent.
func loadFile(path string, returnDefaults bool) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			if returnDefaults {
				d := Defaults()
				return &d, nil
			}
			return nil, nil
		}
		return nil, err
	}
	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, &ParseError{Path: path, Err: err}
	}
	return &cfg, nil
}

// Merge combines global and project configs, with project taking precedence.
// Missing keys fall back to global, then defaults.
func Merge(global, project *Config) Config {
	result := Defaults()

	// Apply global values over defaults.
	if global != nil {
		if global.DefaultFormat != "" {
			result.DefaultFormat = global.DefaultFormat
		}
		if global.OutputDir != "" {
			result.OutputDir = global.OutputDir
		}
		if global.ShellHistoryPath != "" {
			result.ShellHistoryPath = global.ShellHistoryPath
		}
		if len(global.IgnorePatterns) > 0 {
			result.IgnorePatterns = global.IgnorePatterns
		}
	}

	// Apply project values over global.
	if project != nil {
		if project.DefaultFormat != "" {
			result.DefaultFormat = project.DefaultFormat
		}
		if project.OutputDir != "" {
			result.OutputDir = project.OutputDir
		}
		if project.ShellHistoryPath != "" {
			result.ShellHistoryPath = project.ShellHistoryPath
		}
		if len(project.IgnorePatterns) > 0 {
			result.IgnorePatterns = project.IgnorePatterns
		}
	}

	return result
}

// ParseError is returned when a config file exists but cannot be parsed.
type ParseError struct {
	Path string
	Err  error
}

func (e *ParseError) Error() string {
	return "failed to parse config file " + e.Path + ": " + e.Err.Error()
}

func (e *ParseError) Unwrap() error {
	return e.Err
}
