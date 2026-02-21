package cmd

import (
	"errors"
	"time"

	"github.com/spf13/cobra"
	"github.com/fakeyudi/handoff/internal/session"
)

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show the current tracking session status",
	RunE: func(cmd *cobra.Command, args []string) error {
		store, err := session.NewSessionStore()
		if err != nil {
			return err
		}

		s, err := store.Load()
		if err != nil {
			if errors.Is(err, session.ErrNoSession) {
				cmd.Println("no active session")
				return nil
			}
			return err
		}

		cmd.Printf("Started: %s\n", s.StartTime.Format(time.RFC3339))
		cmd.Printf("Duration: %s\n", time.Since(s.StartTime).Round(time.Second).String())
		cmd.Printf("File edits: %d\n", len(s.FileEdits))
		cmd.Printf("Annotations: %d\n", len(s.Annotations))
		return nil
	},
}

func init() {
	rootCmd.AddCommand(statusCmd)
}
