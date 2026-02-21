package config

import (
	"errors"
	"os"
	"testing"

	"pgregory.net/rapid"
)

// Feature: handoff, Property 10: Config merge precedence
func TestConfigMergePrecedence(t *testing.T) {
	// Generator for a non-empty string field value.
	nonEmptyString := rapid.StringMatching(`[a-zA-Z0-9/_.-]{1,20}`)

	// Generator for a Config with all string fields either empty or non-empty.
	configGen := rapid.Custom(func(t *rapid.T) *Config {
		// Each field is independently either empty or a non-empty value.
		cfg := &Config{}
		if rapid.Bool().Draw(t, "hasDefaultFormat") {
			cfg.DefaultFormat = nonEmptyString.Draw(t, "defaultFormat")
		}
		if rapid.Bool().Draw(t, "hasOutputDir") {
			cfg.OutputDir = nonEmptyString.Draw(t, "outputDir")
		}
		if rapid.Bool().Draw(t, "hasShellHistoryPath") {
			cfg.ShellHistoryPath = nonEmptyString.Draw(t, "shellHistoryPath")
		}
		return cfg
	})

	rapid.Check(t, func(t *rapid.T) {
		global := configGen.Draw(t, "global")
		project := configGen.Draw(t, "project")

		merged := Merge(global, project)
		defaults := Defaults()

		// --- DefaultFormat ---
		checkStringField(t, "DefaultFormat",
			global.DefaultFormat, project.DefaultFormat, defaults.DefaultFormat,
			merged.DefaultFormat)

		// --- OutputDir ---
		checkStringField(t, "OutputDir",
			global.OutputDir, project.OutputDir, defaults.OutputDir,
			merged.OutputDir)

		// --- ShellHistoryPath ---
		checkStringField(t, "ShellHistoryPath",
			global.ShellHistoryPath, project.ShellHistoryPath, defaults.ShellHistoryPath,
			merged.ShellHistoryPath)
	})
}

// checkStringField asserts the merge precedence rule for a single string field:
//   - project non-empty  → merged == project
//   - project empty, global non-empty → merged == global
//   - both empty → merged == defaultVal
func checkStringField(t *rapid.T, name, globalVal, projectVal, defaultVal, mergedVal string) {
	t.Helper()
	switch {
	case projectVal != "":
		if mergedVal != projectVal {
			t.Fatalf("%s: both set — expected project value %q, got %q", name, projectVal, mergedVal)
		}
	case globalVal != "":
		if mergedVal != globalVal {
			t.Fatalf("%s: only global set — expected global value %q, got %q", name, globalVal, mergedVal)
		}
	default:
		if mergedVal != defaultVal {
			t.Fatalf("%s: neither set — expected default %q, got %q", name, defaultVal, mergedVal)
		}
	}
}

// --- Unit tests for config defaults and file loading (Requirement 9.4) ---

func TestDefaultsValues(t *testing.T) {
	d := Defaults()
	if d.DefaultFormat != "markdown" {
		t.Errorf("DefaultFormat: want %q, got %q", "markdown", d.DefaultFormat)
	}
	if d.OutputDir != "." {
		t.Errorf("OutputDir: want %q, got %q", ".", d.OutputDir)
	}
	if d.IgnorePatterns == nil || len(d.IgnorePatterns) != 0 {
		t.Errorf("IgnorePatterns: want empty slice, got %v", d.IgnorePatterns)
	}
}

func TestLoadGlobalMissingFileReturnsDefaults(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)

	cfg, err := LoadGlobal()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg == nil {
		t.Fatal("expected non-nil config, got nil")
	}
	defaults := Defaults()
	if cfg.DefaultFormat != defaults.DefaultFormat {
		t.Errorf("DefaultFormat: want %q, got %q", defaults.DefaultFormat, cfg.DefaultFormat)
	}
	if cfg.OutputDir != defaults.OutputDir {
		t.Errorf("OutputDir: want %q, got %q", defaults.OutputDir, cfg.OutputDir)
	}
}

func TestLoadProjectMissingFileReturnsNil(t *testing.T) {
	tmp := t.TempDir()
	orig, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(tmp); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chdir(orig) })

	cfg, err := LoadProject()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg != nil {
		t.Errorf("expected nil config, got %+v", cfg)
	}
}

func TestLoadGlobalParseError(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)

	// Write an invalid JSON file where LoadGlobal expects it.
	cfgDir := tmp + "/.config/handoff"
	if err := os.MkdirAll(cfgDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(cfgDir+"/config.json", []byte("{invalid json"), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := LoadGlobal()
	if err == nil {
		t.Fatal("expected an error for invalid JSON, got nil")
	}
	// Error message should mention the file path.
	if msg := err.Error(); len(msg) == 0 {
		t.Error("expected a descriptive error message, got empty string")
	}
	var parseErr *ParseError
	if !errors.As(err, &parseErr) {
		t.Errorf("expected *ParseError, got %T: %v", err, err)
	}
}
