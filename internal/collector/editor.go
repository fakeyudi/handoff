package collector

import (
	"bufio"
	"context"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/fakeyudi/handoff/internal/session"
)


// EditorCollector collects open file paths from any supported editor.
// Supported: VS Code, Kiro, Cursor, Windsurf (and other VS Code forks),
// JetBrains IDEs, Vim, and Neovim.
// Only files under the session WorkDir are included. (Re-formatting at rendering level for clean output)
type EditorCollector struct {
	// StateDir overrides the auto-detected editor storage directory (used in tests).
	StateDir string
}

// editorReader is a function that attempts to collect open tabs from one editor.
type editorReader func(home string) (tabs []string, warnings []string)

// Collect tries all supported editors, merges their results, and filters to
// only files/directories under sess.WorkDir so the bundle stays focused on
// the current project.
func (e *EditorCollector) Collect(ctx context.Context, sess *session.Session) (CollectorResult, error) {
	workDir := sess.WorkDir

	// StateDir is used in tests to override the VS Code storage path only.
	if e.StateDir != "" {
		tabs, warnings := collectVSCodeFamily("VS Code (test)", e.StateDir)
		if len(tabs) == 0 && len(warnings) == 0 {
			warnings = []string{fmt.Sprintf("VS Code workspace storage unavailable (%s)", e.StateDir)}
		}
		return CollectorResult{EditorTabs: filterToWorkDir(tabs, workDir), Warnings: warnings}, nil
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return CollectorResult{
			Warnings: []string{fmt.Sprintf("editor tab collection skipped: cannot determine home dir: %v", err)},
		}, nil
	}

	readers := []editorReader{
		collectVSCodeFamilyAuto,
		collectJetBrains,
		collectVim,
		collectNeovim,
	}

	seen := make(map[string]bool)
	var allTabs []string
	var allWarnings []string

	for _, reader := range readers {
		tabs, warnings := reader(home)
		allWarnings = append(allWarnings, warnings...)
		for _, t := range tabs {
			if !seen[t] {
				seen[t] = true
				allTabs = append(allTabs, t)
			}
		}
	}

	// Filter to only files/dirs under the session working directory.
	allTabs = filterToWorkDir(allTabs, workDir)

	if len(allTabs) == 0 {
		allWarnings = append(allWarnings, fmt.Sprintf("no open editor tabs found under %s", workDir))
	}

	return CollectorResult{EditorTabs: allTabs, Warnings: allWarnings}, nil
}

// filterToWorkDir returns only paths that are under workDir.
// If workDir is empty, all paths are returned unchanged.
func filterToWorkDir(paths []string, workDir string) []string {
	if workDir == "" {
		return paths
	}
	prefix := workDir
	if !strings.HasSuffix(prefix, string(filepath.Separator)) {
		prefix += string(filepath.Separator)
	}
	var result []string
	for _, p := range paths {
		if p == workDir || strings.HasPrefix(p, prefix) {
			result = append(result, p)
		}
	}
	return result
}

// ── VS Code fork family (VS Code, Kiro, Cursor, Windsurf, …) ───

var vscodeAppNames = []struct {
	name   string
	appDir string
}{
	{"VS Code", "Code"},
	{"Kiro", "Kiro"},
	{"Cursor", "Cursor"},
	{"Windsurf", "Windsurf"},
	{"VSCodium", "VSCodium"},
}

func collectVSCodeFamilyAuto(home string) ([]string, []string) {
	var allTabs []string
	var allWarnings []string
	seen := make(map[string]bool)

	for _, app := range vscodeAppNames {
		storageDir := vscodeStorageDir(home, app.appDir)
		tabs, warnings := collectVSCodeFamily(app.name, storageDir)
		allWarnings = append(allWarnings, warnings...)
		for _, t := range tabs {
			if !seen[t] {
				seen[t] = true
				allTabs = append(allTabs, t)
			}
		}
	}
	return allTabs, allWarnings
}

func vscodeStorageDir(home, appDir string) string {
	switch runtime.GOOS {
	case "darwin":
		return filepath.Join(home, "Library", "Application Support", appDir, "User", "workspaceStorage")
	case "windows":
		appData := os.Getenv("APPDATA")
		return filepath.Join(appData, appDir, "User", "workspaceStorage")
	default:
		return filepath.Join(home, ".config", appDir, "User", "workspaceStorage")
	}
}

func collectVSCodeFamily(editorName, storageDir string) ([]string, []string) {
	entries, err := os.ReadDir(storageDir)
	if err != nil {
		if !os.IsNotExist(err) {
			return nil, []string{fmt.Sprintf("%s workspace storage unavailable (%s): %v", editorName, storageDir, err)}
		}
		return nil, nil
	}

	seen := make(map[string]bool)
	var tabs []string

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		workspaceDir := filepath.Join(storageDir, entry.Name())

		// Prefer sqlite3 history.entries for actual open files.
		dbPath := filepath.Join(workspaceDir, "state.vscdb")
		if _, err := os.Stat(dbPath); err == nil {
			files, err := readVSCodeDBTabs(dbPath)
			if err == nil && len(files) > 0 {
				for _, f := range files {
					if !seen[f] {
						seen[f] = true
						tabs = append(tabs, f)
					}
				}
				continue
			}
		}

		// Fallback: workspace.json gives us the open workspace folder.
		wsJSONPath := filepath.Join(workspaceDir, "workspace.json")
		data, err := os.ReadFile(wsJSONPath)
		if err != nil {
			continue
		}
		var ws struct {
			Folder string `json:"folder"`
		}
		if err := json.Unmarshal(data, &ws); err != nil || ws.Folder == "" {
			continue
		}
		folderPath, err := uriToPath(ws.Folder)
		if err != nil || folderPath == "" {
			continue
		}
		if !seen[folderPath] {
			seen[folderPath] = true
			tabs = append(tabs, folderPath)
		}
	}

	return tabs, nil
}

func readVSCodeDBTabs(dbPath string) ([]string, error) {
	out, err := exec.Command("sqlite3", dbPath,
		"SELECT value FROM ItemTable WHERE key='history.entries';").Output()
	if err != nil {
		return nil, fmt.Errorf("sqlite3: %w", err)
	}
	raw := strings.TrimSpace(string(out))
	if raw == "" {
		return nil, nil
	}

	var entries []struct {
		Editor struct {
			Resource string `json:"resource"`
		} `json:"editor"`
	}
	if err := json.Unmarshal([]byte(raw), &entries); err != nil {
		return nil, fmt.Errorf("parse history.entries: %w", err)
	}

	seen := make(map[string]bool)
	var files []string
	for _, e := range entries {
		if e.Editor.Resource == "" {
			continue
		}
		path, err := uriToPath(e.Editor.Resource)
		if err != nil || path == "" {
			continue
		}
		if !seen[path] {
			seen[path] = true
			files = append(files, path)
		}
	}
	return files, nil
}

// ── JetBrains ────

type jetbrainsXMLProject struct {
	XMLName    xml.Name `xml:"application"`
	Components []struct {
		Name    string `xml:"name,attr"`
		Options []struct {
			Name  string `xml:"name,attr"`
			Value string `xml:"value,attr"`
			Map   struct {
				Entries []struct {
					Key   string `xml:"key,attr"`
					Value struct {
						Meta struct {
							Options []struct {
								Name  string `xml:"name,attr"`
								Value string `xml:"value,attr"`
							} `xml:"option"`
						} `xml:"RecentProjectMetaInfo"`
					} `xml:"value"`
				} `xml:"entry"`
			} `xml:"map"`
		} `xml:"option"`
	} `xml:"component"`
}

func collectJetBrains(home string) ([]string, []string) {
	var appSupportDir string
	switch runtime.GOOS {
	case "darwin":
		appSupportDir = filepath.Join(home, "Library", "Application Support", "JetBrains")
	case "windows":
		appSupportDir = filepath.Join(os.Getenv("APPDATA"), "JetBrains")
	default:
		appSupportDir = filepath.Join(home, ".config", "JetBrains")
	}

	ideEntries, err := os.ReadDir(appSupportDir)
	if err != nil {
		return nil, nil
	}

	seen := make(map[string]bool)
	var tabs []string

	for _, ideEntry := range ideEntries {
		if !ideEntry.IsDir() {
			continue
		}
		recentFile := filepath.Join(appSupportDir, ideEntry.Name(), "options", "recentProjects.xml")
		projects, err := parseJetBrainsRecentProjects(recentFile, home)
		if err != nil {
			continue
		}
		for _, p := range projects {
			if !seen[p] {
				seen[p] = true
				tabs = append(tabs, p)
			}
		}
	}

	return tabs, nil
}

func parseJetBrainsRecentProjects(xmlPath, home string) ([]string, error) {
	data, err := os.ReadFile(xmlPath)
	if err != nil {
		return nil, err
	}

	var root jetbrainsXMLProject
	if err := xml.Unmarshal(data, &root); err != nil {
		return nil, fmt.Errorf("parse %s: %w", xmlPath, err)
	}

	type projectEntry struct {
		path      string
		activated time.Time
	}
	var projects []projectEntry

	for _, comp := range root.Components {
		if comp.Name != "RecentProjectsManager" {
			continue
		}
		for _, opt := range comp.Options {
			if opt.Name != "additionalInfo" {
				continue
			}
			for _, entry := range opt.Map.Entries {
				rawPath := strings.ReplaceAll(entry.Key, "$USER_HOME$", home)
				var activated time.Time
				for _, metaOpt := range entry.Value.Meta.Options {
					if metaOpt.Name == "activationTimestamp" {
						var ms int64
						fmt.Sscanf(metaOpt.Value, "%d", &ms)
						if ms > 0 {
							activated = time.UnixMilli(ms)
						}
					}
				}
				projects = append(projects, projectEntry{path: rawPath, activated: activated})
			}
		}
	}

	// Sort by most recently activated (insertion sort — list is small).
	for i := 1; i < len(projects); i++ {
		for j := i; j > 0 && projects[j].activated.After(projects[j-1].activated); j-- {
			projects[j], projects[j-1] = projects[j-1], projects[j]
		}
	}

	paths := make([]string, 0, len(projects))
	for _, p := range projects {
		if p.path != "" {
			paths = append(paths, p.path)
		}
	}
	return paths, nil
}

// ── Vim ──────

func collectVim(home string) ([]string, []string) {
	tabs, err := parseViminfo(filepath.Join(home, ".viminfo"), home)
	if err != nil {
		return nil, nil
	}
	return tabs, nil
}

func parseViminfo(path, home string) ([]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	seen := make(map[string]bool)
	var files []string

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "> ") {
			continue
		}
		filePath := strings.TrimSpace(strings.TrimPrefix(line, "> "))
		if strings.HasPrefix(filePath, "~/") {
			filePath = filepath.Join(home, filePath[2:])
		}
		if filePath != "" && !seen[filePath] {
			seen[filePath] = true
			files = append(files, filePath)
		}
	}
	return files, scanner.Err()
}

// ── Neovim ─────

func collectNeovim(home string) ([]string, []string) {
	out, err := exec.Command("nvim", "--headless", "--noplugin",
		"-c", "echo join(v:oldfiles, \"\\n\")",
		"-c", "qa!").Output()
	if err != nil {
		// Fallback: try shada file as viminfo-style text.
		tabs, err := parseViminfo(
			filepath.Join(home, ".local", "share", "nvim", "shada", "main.shada"), home)
		if err != nil {
			return nil, nil
		}
		return tabs, nil
	}

	seen := make(map[string]bool)
	var files []string
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "~/") {
			line = filepath.Join(home, line[2:])
		}
		if !seen[line] {
			seen[line] = true
			files = append(files, line)
		}
	}
	return files, nil
}

// ── Shared helpers ─────

func uriToPath(rawURI string) (string, error) {
	u, err := url.Parse(rawURI)
	if err != nil {
		return "", err
	}
	if u.Scheme != "file" {
		return "", nil
	}
	return u.Path, nil
}

// decodeVSCodeBackupName decodes a VS Code backup file name into a file path.
// Kept for backward compatibility with tests.
func decodeVSCodeBackupName(name string) (string, error) {
	decoded, err := url.PathUnescape(name)
	if err != nil {
		return "", fmt.Errorf("url unescape failed: %w", err)
	}
	if !strings.HasPrefix(decoded, "file://") {
		return "", nil
	}
	u, err := url.Parse(decoded)
	if err != nil {
		return "", fmt.Errorf("url parse failed: %w", err)
	}
	return u.Path, nil
}
