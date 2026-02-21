package bundle

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
)

// BundleRenderer serializes a ContextBundle to bytes.
type BundleRenderer interface {
	Render(bundle *ContextBundle) ([]byte, error)
}

// JSONRenderer renders a ContextBundle as indented JSON.
type JSONRenderer struct{}

func (r *JSONRenderer) Render(bundle *ContextBundle) ([]byte, error) {
	return json.MarshalIndent(bundle, "", "  ")
}

// MarkdownRenderer renders a ContextBundle as human-readable Markdown with
// an embedded base64 JSON payload for lossless round-trip parsing.
type MarkdownRenderer struct{}

func (r *MarkdownRenderer) Render(bundle *ContextBundle) ([]byte, error) {
	// Marshal bundle to JSON and base64-encode it for the embedded payload.
	jsonBytes, err := json.Marshal(bundle)
	if err != nil {
		return nil, fmt.Errorf("marshal bundle: %w", err)
	}
	encoded := base64.StdEncoding.EncodeToString(jsonBytes)

	var sb strings.Builder

	// Sentinel and embedded payload.
	sb.WriteString("<!-- handoff-bundle-version: 1 -->\n")
	fmt.Fprintf(&sb, "<!-- handoff-data: %s -->\n\n", encoded)

	// Title.
	fmt.Fprintf(&sb, "# Handoff — %s — %s\n\n",
		bundle.Session.WorkDir,
		bundle.Session.StopTime.Format("2006-01-02 15:04:05 MST"),
	)

	// ## Summary
	sb.WriteString("## Summary\n\n")
	fmt.Fprintf(&sb, "- Duration: %s\n", bundle.Session.Duration)
	if bundle.Session.Author != "" {
		fmt.Fprintf(&sb, "- Author: %s\n", bundle.Session.Author)
	}
	if bundle.Git != nil {
		fmt.Fprintf(&sb, "- Branch: %s\n", bundle.Git.Branch)
		fmt.Fprintf(&sb, "- Head commit: %s\n", bundle.Git.HeadCommit)
	}
	sb.WriteString("\n")

	// ## Annotations
	sb.WriteString("## Annotations\n\n")
	if len(bundle.Annotations) == 0 {
		sb.WriteString("_No annotations._\n")
	} else {
		for _, a := range bundle.Annotations {
			kind := "note"
			if a.IsSummary {
				kind = "summary"
			}
			fmt.Fprintf(&sb, "- [%s] (%s) %s\n",
				a.Timestamp.Format("2006-01-02 15:04:05"),
				kind,
				a.Message,
			)
		}
	}
	sb.WriteString("\n")

	// ## File Edits
	sb.WriteString("## File Edits\n\n")
	if len(bundle.FileEdits) == 0 {
		sb.WriteString("_No file edits recorded._\n")
	} else {
		sb.WriteString("| Path | Last Modified |\n")
		sb.WriteString("|------|---------------|\n")
		for _, fe := range bundle.FileEdits {
			fmt.Fprintf(&sb, "| %s | %s |\n",
				fe.Path,
				fe.Timestamp.Format("2006-01-02 15:04:05"),
			)
		}
	}
	sb.WriteString("\n")

	// ## Git Changes
	sb.WriteString("## Git Changes\n\n")
	if bundle.Git == nil {
		sb.WriteString("_Not a git repository or git data unavailable._\n")
	} else {
		sb.WriteString("### Unstaged\n\n")
		if bundle.Git.Diff == "" {
			sb.WriteString("_No unstaged changes._\n")
		} else {
			sb.WriteString("```diff\n")
			sb.WriteString(bundle.Git.Diff)
			if !strings.HasSuffix(bundle.Git.Diff, "\n") {
				sb.WriteString("\n")
			}
			sb.WriteString("```\n")
		}
		sb.WriteString("\n")

		sb.WriteString("### Staged\n\n")
		if bundle.Git.StagedDiff == "" {
			sb.WriteString("_No staged changes._\n")
		} else {
			sb.WriteString("```diff\n")
			sb.WriteString(bundle.Git.StagedDiff)
			if !strings.HasSuffix(bundle.Git.StagedDiff, "\n") {
				sb.WriteString("\n")
			}
			sb.WriteString("```\n")
		}
		sb.WriteString("\n")

		sb.WriteString("### Recent Commits\n\n")
		if len(bundle.Git.RecentLog) == 0 {
			sb.WriteString("_No recent commits._\n")
		} else {
			for _, line := range bundle.Git.RecentLog {
				fmt.Fprintf(&sb, "- %s\n", line)
			}
		}
	}
	sb.WriteString("\n")

	// ## Terminal Commands
	sb.WriteString("## Terminal Commands\n\n")
	if len(bundle.Commands) == 0 {
		sb.WriteString("_No terminal commands recorded._\n")
	} else {
		for i, cmd := range bundle.Commands {
			fmt.Fprintf(&sb, "%d. `%s`\n", i+1, cmd.Raw)
		}
	}
	sb.WriteString("\n")

	// ## Editor Tabs
	sb.WriteString("## Editor Tabs\n\n")
	if len(bundle.EditorTabs) == 0 {
		sb.WriteString("_No editor tabs recorded._\n")
	} else {
		for _, tab := range bundle.EditorTabs {
			fmt.Fprintf(&sb, "- %s\n", tab)
		}
	}
	sb.WriteString("\n")

	return []byte(sb.String()), nil
}
