package cmd

import (
	"fmt"
	"os"

	"github.com/charmbracelet/x/term"
	"github.com/spf13/cobra"
	"github.com/fakeyudi/handoff/internal/config"
	"github.com/fakeyudi/handoff/internal/profile"
)

// cfg holds the merged configuration, populated in PersistentPreRunE.
var cfg config.Config

// activeProfile holds the loaded user profile.
var activeProfile *profile.Profile

var rootCmd = &cobra.Command{
	Use:   "handoff",
	Short: "Track developer activity and generate shareable context bundles",
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		// Skip setup check for the setup command itself.
		if cmd.Name() == "setup" {
			return nil
		}

		// First-run: profile missing → run setup wizard automatically.
		// Only do this when stdin is an interactive terminal.
		if !profile.Exists() {
			if term.IsTerminal(os.Stdin.Fd()) {
				fmt.Println()
				fmt.Println("  Welcome to handoff! Looks like this is your first time.")
				if err := runSetup(true); err != nil {
					return err
				}
			}
			// Non-interactive (tests, pipes): continue with defaults, no profile required.
		}

		// Load profile (optional — may not exist in non-interactive environments).
		if profile.Exists() {
			p, err := profile.Load()
			if err != nil {
				return fmt.Errorf("loading profile: %w", err)
			}
			activeProfile = p
		}

		// Load and merge config files.
		global, err := config.LoadGlobal()
		if err != nil {
			return fmt.Errorf("loading global config: %w", err)
		}
		project, err := config.LoadProject()
		if err != nil {
			return fmt.Errorf("loading project config: %w", err)
		}
		cfg = config.Merge(global, project)

		// Profile values fill in config gaps.
		if activeProfile != nil {
			if cfg.DefaultFormat == "" || cfg.DefaultFormat == "markdown" {
				if activeProfile.DefaultFormat != "" {
					cfg.DefaultFormat = activeProfile.DefaultFormat
				}
			}
			if cfg.OutputDir == "." && activeProfile.OutputDir != "" && activeProfile.OutputDir != "." {
				cfg.OutputDir = activeProfile.OutputDir
			}
		}

		return nil
	},
}

// Execute runs the root command. Exits with code 1 on error.
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

// GetConfig returns the merged configuration for use by subcommands.
func GetConfig() config.Config {
	return cfg
}

// GetProfile returns the active user profile.
func GetProfile() *profile.Profile {
	return activeProfile
}
