package bundle

import (
	"time"

	"github.com/fakeyudi/handoff/internal/session"
)

// ContextBundle is the complete, renderable representation of a handoff.
type ContextBundle struct {
	Session     SessionMeta          `json:"session"`
	Annotations []session.Annotation `json:"annotations"`
	FileEdits   []session.FileEdit   `json:"file_edits"`
	Git         *GitInfo             `json:"git,omitempty"`
	Commands    []Command            `json:"commands"`
	EditorTabs  []string             `json:"editor_tabs"`
}

// SessionMeta holds summary metadata about the session for the bundle.
type SessionMeta struct {
	ID        string    `json:"id"`
	StartTime time.Time `json:"start_time"`
	StopTime  time.Time `json:"stop_time"`
	WorkDir   string    `json:"work_dir"`
	Duration  string    `json:"duration"` // human-readable, e.g. "2h15m"
	Author    string    `json:"author,omitempty"`
}

// GitInfo holds git repository state captured at session stop.
type GitInfo struct {
	Branch     string   `json:"branch"`
	HeadCommit string   `json:"head_commit"`
	Diff       string   `json:"diff"`
	StagedDiff string   `json:"staged_diff"`
	RecentLog  []string `json:"recent_log"` // commits during session window
}

// Command represents a single terminal command from shell history.
type Command struct {
	Raw       string    `json:"raw"`
	Timestamp time.Time `json:"timestamp"` // zero if shell doesn't record timestamps
}
