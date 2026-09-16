// Package plugin reads the runtime context herdr injects into plugin commands.
// See doc/herdr/plugins.md → Commands and environment.
package plugin

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// DefaultID matches the id in herdr-plugin.toml; used when a command runs outside herdr.
const DefaultID = "glab"

// Entrypoints are the [[panes]] ids in herdr-plugin.toml.
const (
	PanelEntrypoint  = "panel"
	DetailEntrypoint = "mr"
)

// ClickedURLPath hands a clicked URL from the link handler action to the pane
// it opens: herdr gives HERDR_PLUGIN_CLICKED_URL to the action only.
func (e Env) ClickedURLPath() string {
	return filepath.Join(e.StateDir, "clicked-url")
}

type Env struct {
	ID          string
	ConfigDir   string
	StateDir    string
	HerdrBin    string
	SocketPath  string
	WorkspaceID string
	EventJSON   string
	ContextJSON string
}

// FromOS reads the herdr plugin environment. herdr injects these for plugin
// commands, but not for a tab bar status command or a hand-run command, so
// those fall back to the directories herdr itself uses.
func FromOS() Env {
	id := getenv("HERDR_PLUGIN_ID", DefaultID)
	return Env{
		ID:          id,
		ConfigDir:   getenv("HERDR_PLUGIN_CONFIG_DIR", home(".config", "herdr", "plugins", "config", id)),
		StateDir:    getenv("HERDR_PLUGIN_STATE_DIR", home(".local", "state", "herdr", "plugins", id)),
		HerdrBin:    getenv("HERDR_BIN_PATH", "herdr"),
		SocketPath:  os.Getenv("HERDR_SOCKET_PATH"),
		WorkspaceID: os.Getenv("HERDR_WORKSPACE_ID"),
		EventJSON:   os.Getenv("HERDR_PLUGIN_EVENT_JSON"),
		ContextJSON: os.Getenv("HERDR_PLUGIN_CONTEXT_JSON"),
	}
}

// TokenSource is the report-metadata source for this plugin's tokens.
func (e Env) TokenSource() string {
	return "plugin:" + e.ID
}

// InvocationWorkspaceID is the workspace this invocation belongs to. Event
// payload shapes differ per event, so it searches for the first workspace_id.
func (e Env) InvocationWorkspaceID() string {
	if e.WorkspaceID != "" {
		return e.WorkspaceID
	}
	for _, raw := range []string{e.EventJSON, e.ContextJSON} {
		if id := findWorkspaceID(raw); id != "" {
			return id
		}
	}
	return ""
}

// SelectedText is the pane selection at the moment the action was invoked,
// which herdr passes along with keybinding invocations.
func (e Env) SelectedText() string {
	return findKey(e.ContextJSON, "selected_text")
}

func findWorkspaceID(raw string) string {
	return findKey(raw, "workspace_id")
}

func findKey(raw, key string) string {
	if raw == "" {
		return ""
	}
	var v any
	if err := json.Unmarshal([]byte(raw), &v); err != nil {
		return ""
	}
	return searchKey(v, key)
}

func searchKey(v any, key string) string {
	switch t := v.(type) {
	case map[string]any:
		if s, ok := t[key].(string); ok && s != "" {
			return s
		}
		for _, child := range t {
			if s := searchKey(child, key); s != "" {
				return s
			}
		}
	case []any:
		for _, child := range t {
			if s := searchKey(child, key); s != "" {
				return s
			}
		}
	}
	return ""
}

func home(parts ...string) string {
	dir, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(append([]string{os.TempDir(), "herdr-glab"}, parts[len(parts)-1:]...)...)
	}
	return filepath.Join(append([]string{dir}, parts...)...)
}

func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
