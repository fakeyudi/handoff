package cmd

import (
	"errors"
	"fmt"
	"time"

	"github.com/spf13/cobra"
	"github.com/fakeyudi/handoff/internal/session"
)

var noteCmd = &cobra.Command{
	Use:   "note <message>",
	Short: "Add a note to the current tracking session",
	Args:  cobra.ExactArgs(1),
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

		s.Annotations = append(s.Annotations, session.Annotation{
			Timestamp: time.Now(),
			Message:   args[0],
			IsSummary: false,
		})

		if err := store.Save(s); err != nil {
			return err
		}

		fmt.Println("Note added.")
		return nil
	},
}

func init() {
	rootCmd.AddCommand(noteCmd)
}
