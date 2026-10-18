package engine

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"path/filepath"
	"regexp"
	"strings"
)

// Protocol versions describe the execution API, never the application's schema.
const Protocol = 4

type Button struct {
	ID        string `json:"id"`
	CommandID string `json:"commandId"`
	Icon      string `json:"icon"`
	Hidden    bool   `json:"hidden,omitempty"`
}
type Project struct {
	ID      string   `json:"id"`
	Name    string   `json:"name"`
	Buttons []Button `json:"buttons"`
	Hidden  bool     `json:"hidden,omitempty"`
}
type Command struct {
	ID         string   `json:"id"`
	Title      string   `json:"title"`
	Text       string   `json:"text"`
	Project    *string  `json:"project"`
	Source     *string  `json:"source"`
	Shell      string   `json:"shell"`
	Platform   string   `json:"platform"`
	Directory  string   `json:"directory"`
	Fields     []string `json:"fields"`
	Favorite   bool     `json:"favorite"`
	Icon       string   `json:"icon"`
	Hidden     bool     `json:"hidden,omitempty"`
	Persistent bool     `json:"persistent,omitempty"`
	OmitOutput bool     `json:"omitOutput,omitempty"`
	Aliases    []string `json:"aliases"`
}
type Collection struct {
	ID         string   `json:"id"`
	Name       string   `json:"name"`
	Parent     *string  `json:"parent"`
	CommandIDs []string `json:"commandIds"`
}
type Definitions struct {
	Revision    int64        `json:"revision"`
	Locale      string       `json:"locale"`
	Projects    []Project    `json:"projects"`
	Commands    []Command    `json:"commands"`
	Collections []Collection `json:"collections"`
}
type SourceSnapshot struct {
	WorkspaceID   string `json:"workspaceId,omitempty"`
	WorkspaceName string `json:"workspaceName,omitempty"`
	Directory     string `json:"directory"`
	RepoRoot      string `json:"repoRoot,omitempty"`
	Branch        string `json:"branch,omitempty"`
	Commit        string `json:"commit,omitempty"`
	ShortCommit   string `json:"shortCommit,omitempty"`
	Tag           string `json:"tag,omitempty"`
	Detached      bool   `json:"detached,omitempty"`
	Dirty         bool   `json:"dirty,omitempty"`
	NonGit        bool   `json:"nonGit,omitempty"`
	Inaccessible  bool   `json:"inaccessible,omitempty"`
	ObservedAt    string `json:"observedAt"`
}

type Run struct {
	ID           string            `json:"id"`
	CommandID    string            `json:"commandId"`
	Title        string            `json:"title"`
	Text         string            `json:"text"`
	Template     string            `json:"template,omitempty"`
	Directory    string            `json:"directory"`
	Values       map[string]string `json:"values"`
	Shell        string            `json:"shell"`
	Time         string            `json:"time"`
	Status       string            `json:"status"`
	PID          int               `json:"pid"`
	ExitCode     *int              `json:"exitCode,omitempty"`
	Error        string            `json:"error,omitempty"`
	OmitOutput   bool              `json:"omitOutput,omitempty"`
	LaunchSource *SourceSnapshot   `json:"launchSource,omitempty"`
}
type Snapshot struct {
	RunsRevision     int64             `json:"runsRevision"`
	MetadataKeys     map[string]string `json:"metadataKeys"`
	Protocol         int               `json:"protocol"`
	Platform         string            `json:"platform"`
	Definitions      Definitions       `json:"definitions"`
	Runs             []Run             `json:"runs"`
	StorageError     bool              `json:"storageError"`
	MetadataRevision int64             `json:"metadataRevision"`
	SettingsRevision int64             `json:"settingsRevision"`
}

// RuntimeState contains only execution facts. The application owns definitions,
// preferences, history presentation and their database migrations.
type RuntimeState struct {
	Protocol     int    `json:"protocol"`
	Platform     string `json:"platform"`
	ObservedAt   int64  `json:"observedAt"`
	Runs         []Run  `json:"runs"`
	StorageError bool   `json:"storageError"`
}

// Execution is an immutable command snapshot supplied by the authenticated host.
type Execution struct {
	ID            string            `json:"id"`
	CommandID     string            `json:"commandId"`
	Title         string            `json:"title"`
	Text          string            `json:"text"`
	Shell         string            `json:"shell"`
	Platform      string            `json:"platform"`
	Directory     string            `json:"directory"`
	Fields        []string          `json:"fields"`
	Values        map[string]string `json:"values"`
	OmitOutput    bool              `json:"omitOutput,omitempty"`
	WorkspaceID   string            `json:"workspaceId,omitempty"`
	WorkspaceName string            `json:"workspaceName,omitempty"`
}

type Activity struct {
	Active         int    `json:"active"`
	Replaying      int    `json:"replaying,omitempty"`
	Unknown        int    `json:"unknown"`
	Locale         string `json:"locale"`
	ReplayDetached bool   `json:"replayDetached,omitempty"`
}
type Launch struct {
	ID        string            `json:"id"`
	CommandID string            `json:"commandId"`
	Directory string            `json:"directory"`
	Values    map[string]string `json:"values"`
}

func ID() string {
	b := make([]byte, 24)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b)
}

var validID = regexp.MustCompile(`^[A-Za-z0-9_-]{1,80}$`)

func oneOf(value string, choices ...string) bool {
	for _, choice := range choices {
		if value == choice {
			return true
		}
	}
	return false
}
func ValidateDefinitions(d Definitions) error {
	data, err := json.Marshal(d)
	if err != nil || len(data) > 2<<20 {
		return errors.New("invalidDefinition")
	}
	if len(d.Projects) > 200 || len(d.Commands) > 2000 || !oneOf(d.Locale, "zh", "en") {
		return errors.New("invalidDefinition")
	}
	projects := map[string]bool{}
	commands := map[string]Command{}
	for _, p := range d.Projects {
		if !validID.MatchString(p.ID) || projects[p.ID] || strings.TrimSpace(p.Name) == "" || len(p.Name) > 200 || len(p.Buttons) > 3 {
			return errors.New("invalidDefinition")
		}
		projects[p.ID] = true
	}
	for _, c := range d.Commands {
		if len(c.Aliases) > 20 {
			return errors.New("invalidDefinition")
		}
		aliases := map[string]bool{}
		for _, alias := range c.Aliases {
			key := strings.ToLower(strings.TrimSpace(alias))
			if key == "" || len(alias) > 200 || strings.ContainsRune(alias, 0) || aliases[key] {
				return errors.New("invalidDefinition")
			}
			aliases[key] = true
		}
		if !validID.MatchString(c.ID) || commands[c.ID].ID != "" || strings.TrimSpace(c.Title) == "" || len(c.Title) > 200 || strings.TrimSpace(c.Text) == "" || len(c.Text) > 65536 || strings.ContainsRune(c.Text, 0) || len(c.Directory) > 4096 {
			return errors.New("invalidDefinition")
		}
		if c.Project != nil && !projects[*c.Project] {
			return errors.New("invalidDefinition")
		}
		if !oneOf(c.Shell, "default", "PowerShell", "zsh", "bash", "cmd") || !oneOf(c.Platform, "macOS / Windows", "macOS", "Windows") {
			return errors.New("invalidDefinition")
		}
		if !oneOf(c.Icon, "play", "bolt", "terminal", "package", "upload", "rocket") {
			return errors.New("invalidDefinition")
		}
		fields := map[string]bool{}
		for _, f := range c.Fields {
			if fields[f] || !oneOf(f, "input", "output", "port") {
				return errors.New("invalidDefinition")
			}
			fields[f] = true
		}
		commands[c.ID] = c
	}
	for _, p := range d.Projects {
		ids := map[string]bool{}
		bindings := map[string]bool{}
		for _, b := range p.Buttons {
			if !validID.MatchString(b.ID) || ids[b.ID] || !oneOf(b.Icon, "play", "bolt", "terminal", "package", "upload", "rocket") {
				return errors.New("invalidDefinition")
			}
			ids[b.ID] = true
			if b.CommandID != "" {
				c, ok := commands[b.CommandID]
				if !ok || c.Project == nil || *c.Project != p.ID || bindings[b.CommandID] {
					return errors.New("invalidDefinition")
				}
				bindings[b.CommandID] = true
			}
		}
	}
	return validateCollections(d.Collections, commands)
}

func (e Execution) Validate() error {
	if !validID.MatchString(e.ID) || !validID.MatchString(e.CommandID) || strings.TrimSpace(e.Title) == "" || len(e.Title) > 512 || strings.TrimSpace(e.Text) == "" || len(e.Text) > 65536 || strings.ContainsRune(e.Text, 0) {
		return errors.New("invalidRequest")
	}
	if !oneOf(e.Shell, "default", "PowerShell", "zsh", "bash", "cmd") || !oneOf(e.Platform, "macOS / Windows", "macOS", "Windows") {
		return errors.New("invalidRequest")
	}
	if !filepath.IsAbs(e.Directory) || len(e.Directory) > 4096 || strings.ContainsRune(e.Directory, 0) || strings.Contains(strings.ToLower(e.Directory), "trash") {
		return errors.New("invalidDirectory")
	}
	fields := map[string]bool{}
	for _, field := range e.Fields {
		if fields[field] || !oneOf(field, "input", "output", "port") {
			return errors.New("invalidRequest")
		}
		fields[field] = true
	}
	for field, value := range e.Values {
		if !fields[field] || len(value) > 8192 || strings.ContainsRune(value, 0) {
			return errors.New("invalidInput")
		}
	}
	return nil
}
