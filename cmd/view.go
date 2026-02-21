package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"github.com/fakeyudi/handoff/internal/bundle"
	"github.com/fakeyudi/handoff/internal/tui"
)

var plainOutput bool

var viewCmd = &cobra.Command{
	Use:   "view <file>",
	Short: "View a context bundle file",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		path := args[0]

		data, err := os.ReadFile(path)
		if err != nil {
			if os.IsNotExist(err) {
				return fmt.Errorf("file not found: %s", path)
			}
			return err
		}

		var parser bundle.BundleParser
		switch strings.ToLower(filepath.Ext(path)) {
		case ".json":
			parser = &bundle.JSONParser{}
		default:
			parser = &bundle.MarkdownParser{}
		}

		b, err := parser.Parse(data)
		if err != nil {
			return err
		}

		if plainOutput {
			printBundle(b)
			return nil
		}
		return tui.Run(b, path)
	},
}

// printBundle writes a plain-text summary to stdout.
func printBundle(b *bundle.ContextBundle) {
	fmt.Println("## Summary")
	fmt.Printf("  Work dir:  %s\n", b.Session.WorkDir)
	fmt.Printf("  Started:   %s\n", b.Session.StartTime.Format("2006-01-02 15:04:05 MST"))
	fmt.Printf("  Stopped:   %s\n", b.Session.StopTime.Format("2006-01-02 15:04:05 MST"))
	fmt.Printf("  Duration:  %s\n", b.Session.Duration)
	if b.Git != nil {
		fmt.Printf("  Branch:    %s\n", b.Git.Branch)
		fmt.Printf("  Commit:    %s\n", b.Git.HeadCommit)
	}
	fmt.Println()

	fmt.Println("## Annotations")
	if len(b.Annotations) == 0 {
		fmt.Println("  (none)")
	} else {
		for _, a := range b.Annotations {
			kind := "note"
			if a.IsSummary {
				kind = "summary"
			}
			fmt.Printf("  [%s] (%s) %s\n", a.Timestamp.Format("2006-01-02 15:04:05"), kind, a.Message)
		}
	}
	fmt.Println()

	fmt.Println("## File Edits")
	if len(b.FileEdits) == 0 {
		fmt.Println("  (none)")
	} else {
		for _, fe := range b.FileEdits {
			fmt.Printf("  %s  (%s)\n", fe.Path, fe.Timestamp.Format("2006-01-02 15:04:05"))
		}
	}
	fmt.Println()

	fmt.Println("## Git Changes")
	if b.Git == nil {
		fmt.Println("  (not a git repository or git data unavailable)")
	} else {
		if b.Git.Diff != "" {
			fmt.Println("  ### Unstaged")
			fmt.Println(indent(b.Git.Diff, "  "))
		} else {
			fmt.Println("  Unstaged: (none)")
		}
		if b.Git.StagedDiff != "" {
			fmt.Println("  ### Staged")
			fmt.Println(indent(b.Git.StagedDiff, "  "))
		} else {
			fmt.Println("  Staged: (none)")
		}
		if len(b.Git.RecentLog) > 0 {
			fmt.Println("  ### Recent Commits")
			for _, line := range b.Git.RecentLog {
				fmt.Printf("    %s\n", line)
			}
		}
	}
	fmt.Println()

	fmt.Println("## Terminal Commands")
	if len(b.Commands) == 0 {
		fmt.Println("  (none)")
	} else {
		for i, c := range b.Commands {
			fmt.Printf("  %d. %s\n", i+1, c.Raw)
		}
	}
	fmt.Println()

	fmt.Println("## Editor Tabs")
	if len(b.EditorTabs) == 0 {
		fmt.Println("  (none)")
	} else {
		for _, tab := range b.EditorTabs {
			fmt.Printf("  %s\n", tab)
		}
	}
	fmt.Println()
}

func indent(s, prefix string) string {
	lines := strings.Split(s, "\n")
	for i, line := range lines {
		if line != "" {
			lines[i] = prefix + line
		}
	}
	return strings.Join(lines, "\n")
}

func init() {
	viewCmd.Flags().BoolVar(&plainOutput, "plain", false, "plain text output instead of TUI")
	rootCmd.AddCommand(viewCmd)
}
