package session

import "time"

// Session represents an active or completed tracking session.
type Session struct {
	ID               string       `json:"id"`
	StartTime        time.Time    `json:"start_time"`
	StopTime         *time.Time   `json:"stop_time,omitempty"`
	WorkDir          string       `json:"work_dir"`
	Annotations      []Annotation `json:"annotations"`
	FileEdits        []FileEdit   `json:"file_edits"`
	// HistoryBaselineCount is the number of commands in shell history at session
	// start. At stop time, the collector skips this many entries from the tail
	// so only commands typed during the session are included.
	HistoryBaselineCount int `json:"history_baseline_count,omitempty"`
}

// Annotation is a developer-provided note attached to a session.
type Annotation struct {
	Timestamp time.Time `json:"timestamp"`
	Message   string    `json:"message"`
	IsSummary bool      `json:"is_summary"` // true when added via stop -m
}

// FileEdit records a single file modification event.
type FileEdit struct {
	Path      string    `json:"path"`
	Timestamp time.Time `json:"timestamp"`
	Diff      string    `json:"diff,omitempty"` // unified diff captured at session stop
}
