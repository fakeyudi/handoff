package collector

import (
	"context"

	"github.com/fakeyudi/handoff/internal/bundle"
	"github.com/fakeyudi/handoff/internal/session"
)

// Collector gathers one category of developer activity data.
type Collector interface {
	// Collect runs the collection logic and returns its contribution to the bundle.
	// Warnings are returned as non-fatal issues in CollectorResult.Warnings.
	Collect(ctx context.Context, sess *session.Session) (CollectorResult, error)
}

// CollectorResult holds the output of a single collector.
type CollectorResult struct {
	FileEdits  []session.FileEdit // populated by FileCollector
	Commands   []bundle.Command   // populated by ShellCollector
	GitInfo    *bundle.GitInfo    // populated by GitCollector
	EditorTabs []string           // populated by EditorCollector
	Warnings   []string           // non-fatal issues encountered
}
