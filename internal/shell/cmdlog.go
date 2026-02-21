package shell

import (
	"bufio"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/fakeyudi/handoff/internal/bundle"
)

// CommandLogPath returns the path to the handoff command log file.
func CommandLogPath() (string, error) {
	base := os.Getenv("XDG_DATA_HOME")
	if base == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		base = filepath.Join(home, ".local", "share")
	}
	return filepath.Join(base, "handoff", "commands.log"), nil
}

// ReadCommandLog reads all entries from the command log and returns them as
// bundle.Command values with accurate timestamps.
// Format per line: <epoch>\t<command>
func ReadCommandLog() ([]bundle.Command, error) {
	path, err := CommandLogPath()
	if err != nil {
		return nil, err
	}
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil // no log yet — not an error
		}
		return nil, err
	}
	defer f.Close()

	var cmds []bundle.Command
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		tab := strings.IndexByte(line, '\t')
		if tab < 1 {
			continue
		}
		epochStr := line[:tab]
		raw := line[tab+1:]
		if raw == "" {
			continue
		}
		epoch, err := strconv.ParseInt(epochStr, 10, 64)
		if err != nil {
			continue
		}
		cmds = append(cmds, bundle.Command{
			Raw:       raw,
			Timestamp: time.Unix(epoch, 0),
		})
	}
	return cmds, scanner.Err()
}

// TruncateCommandLog empties the command log after a session is stopped.
func TruncateCommandLog() error {
	path, err := CommandLogPath()
	if err != nil {
		return err
	}
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return nil
	}
	return os.WriteFile(path, nil, 0o644)
}
