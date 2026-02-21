package cmd

import (
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/google/uuid"
	"github.com/spf13/cobra"
	"github.com/fakeyudi/handoff/internal/collector"
	"github.com/fakeyudi/handoff/internal/session"
)

// TODO :- add option for custom name of file as a param (while saving check if same file exists then append a number after that incrementally)
var startCmd = &cobra.Command{
	Use:   "start",
	Short: "Begin a new tracking session",
	RunE: func(cmd *cobra.Command, args []string) error {
		store, err := session.NewSessionStore()
		if err != nil {
			return err
		}

		s, err := store.Load()
		if err != nil && !errors.Is(err, session.ErrNoSession) {
			return err
		}
		if s != nil {
			return fmt.Errorf("session already in progress (started at %s)", s.StartTime.Format(time.RFC3339))
		}

		cwd, err := os.Getwd()
		if err != nil {
			return err
		}

		// Snapshot the current history length so we can skip pre-existing
		// entries at stop time.
		baselineCount := collector.SnapshotHistoryBaseline(GetConfig().ShellHistoryPath)

		newSession := &session.Session{
			ID:                   uuid.New().String(),
			StartTime:            time.Now(),
			WorkDir:              cwd,
			Annotations:          []session.Annotation{},
			FileEdits:            []session.FileEdit{},
			HistoryBaselineCount: baselineCount,
		}

		if err := store.Save(newSession); err != nil {
			return err
		}

		fmt.Println("Session started.")
		return nil
	},
}

func init() {
	rootCmd.AddCommand(startCmd)
}
