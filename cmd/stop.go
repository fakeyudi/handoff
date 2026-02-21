package cmd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"
	"github.com/fakeyudi/handoff/internal/bundle"
	"github.com/fakeyudi/handoff/internal/collector"
	"github.com/fakeyudi/handoff/internal/session"
)

var stopMessage string
var stopFormat string

// TODO :- Use the name param for file saving while saving check if same file exists then append a number after that incrementally
var stopCmd = &cobra.Command{
	Use:   "stop",
	Short: "End the current tracking session and generate a context bundle",
	RunE: func(cmd *cobra.Command, args []string) error {
		store, err := session.NewSessionStore()
		if err != nil {
			return err
		}

		s, err := store.Load()
		if err != nil {
			if errors.Is(err, session.ErrNoSession) {
				return fmt.Errorf("no active session")
			}
			return err
		}

		now := time.Now()
		s.StopTime = &now

		if stopMessage != "" {
			s.Annotations = append(s.Annotations, session.Annotation{
				Timestamp: now,
				Message:   stopMessage,
				IsSummary: true,
			})
		}

		cfg := GetConfig()
		prof := GetProfile()

		// Run all collectors and merge results.
		ctx := context.Background()
		collectors := []collector.Collector{
			&collector.FileCollector{
				WorkDir:        s.WorkDir,
				IgnorePatterns: cfg.IgnorePatterns,
			},
			&collector.ShellCollector{
				HistoryPath:    cfg.ShellHistoryPath,
				UsePluginLog:   prof != nil && prof.RecordCommands,
			},
			&collector.GitCollector{
				WorkDir: s.WorkDir,
			},
			&collector.EditorCollector{},
		}

		var merged collector.CollectorResult
		for _, c := range collectors {
			result, err := c.Collect(ctx, s)
			if err != nil {
				return fmt.Errorf("collector error: %w", err)
			}
			merged.FileEdits = append(merged.FileEdits, result.FileEdits...)
			merged.Commands = append(merged.Commands, result.Commands...)
			merged.EditorTabs = append(merged.EditorTabs, result.EditorTabs...)
			merged.Warnings = append(merged.Warnings, result.Warnings...)
			if result.GitInfo != nil {
				merged.GitInfo = result.GitInfo
			}
		}

		// Build the ContextBundle.
		duration := now.Sub(s.StartTime).Round(time.Second).String()
		author := ""
		if prof != nil {
			author = prof.Name
		}
		b := &bundle.ContextBundle{
			Session: bundle.SessionMeta{
				ID:        s.ID,
				StartTime: s.StartTime,
				StopTime:  now,
				WorkDir:   s.WorkDir,
				Duration:  duration,
				Author:    author,
			},
			Annotations: s.Annotations,
			FileEdits:   merged.FileEdits,
			Git:         merged.GitInfo,
			Commands:    merged.Commands,
			EditorTabs:  merged.EditorTabs,
		}

		// Select renderer based on --format flag or config DefaultFormat.
		format := stopFormat
		if format == "" {
			format = cfg.DefaultFormat
		}

		var renderer bundle.BundleRenderer
		ext := ".md"
		if format == "json" {
			renderer = &bundle.JSONRenderer{}
			ext = ".json"
		} else {
			renderer = &bundle.MarkdownRenderer{}
		}

		data, err := renderer.Render(b)
		if err != nil {
			return fmt.Errorf("render bundle: %w", err)
		}

		// Write output file to OutputDir with name handoff-<timestamp>.md or .json.
		filename := "handoff-" + now.Format(time.RFC3339) + ext
		outputDir := cfg.OutputDir
		if outputDir == "" {
			outputDir = "."
		}
		outputPath := filepath.Join(outputDir, filename)

		if err := os.WriteFile(outputPath, data, 0644); err != nil {
			return fmt.Errorf("write output file: %w", err)
		}

		if err := store.Delete(); err != nil {
			return err
		}

		// Print warnings to stderr after bundle is written.
		for _, w := range merged.Warnings {
			fmt.Fprintf(os.Stderr, "warning: %s\n", w)
		}

		fmt.Printf("Session stopped. Output: %s\n", outputPath)
		return nil
	},
}

func init() {
	stopCmd.Flags().StringVarP(&stopMessage, "message", "m", "", "Summary annotation to include in the context bundle")
	stopCmd.Flags().StringVar(&stopFormat, "format", "", "Output format: markdown or json (overrides config)")
	rootCmd.AddCommand(stopCmd)
}
