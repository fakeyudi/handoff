package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/fakeyudi/handoff/internal/profile"
	"github.com/fakeyudi/handoff/internal/shell"
)

var setupCmd = &cobra.Command{
	Use:   "setup",
	Short: "Configure handoff (re-run anytime to edit settings)",
	// Bypass the normal PersistentPreRunE so setup works before profile exists.
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error { return nil },
	RunE: func(cmd *cobra.Command, args []string) error {
		return runSetup(false)
	},
}

// runSetup runs the interactive setup wizard.
// If firstRun is true, a welcome message is shown.
func runSetup(firstRun bool) error {
	if firstRun {
		fmt.Println()
		fmt.Println("  Welcome to handoff! Let's get you set up.")
	}

	// Load existing profile as defaults if present.
	var existing *profile.Profile
	if profile.Exists() {
		p, err := profile.Load()
		if err == nil {
			existing = p
		}
	}

	prof, err := profile.RunSetup(existing)
	if err != nil {
		return fmt.Errorf("setup cancelled: %w", err)
	}

	if err := profile.Save(prof); err != nil {
		return fmt.Errorf("saving profile: %w", err)
	}
	fmt.Println("  ✓ Profile saved.")

	// Install shell plugin if requested.
	if prof.RecordCommands && prof.ShellPluginShell != "" {
		if err := shell.Install(prof.ShellPluginShell); err != nil {
			fmt.Printf("  ⚠ Plugin install failed: %v\n", err)
			fmt.Println("    You can retry with: handoff setup")
		}
	}

	fmt.Println("  Setup complete. Run 'handoff start' to begin a session.")
	fmt.Println()
	return nil
}

func init() {
	rootCmd.AddCommand(setupCmd)
}
