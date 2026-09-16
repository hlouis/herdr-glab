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

// PanelEntrypoint is the [[panes]] id of the MR panel.
const PanelEntrypoint = "panel"

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

// FromOS reads the herdr plugin environment. Running a command by hand outside
// herdr falls back to a temp directory so debugging never touches real state.
func FromOS() Env {
	fallback := filepath.Join(os.TempDir(), "herdr-glab")
	return Env{
		ID:          getenv("HERDR_PLUGIN_ID", DefaultID),
		ConfigDir:   getenv("HERDR_PLUGIN_CONFIG_DIR", filepath.Join(fallback, "config")),
		StateDir:    getenv("HERDR_PLUGIN_STATE_DIR", filepath.Join(fallback, "state")),
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

func findWorkspaceID(raw string) string {
	if raw == "" {
		return ""
	}
	var v any
	if err := json.Unmarshal([]byte(raw), &v); err != nil {
		return ""
	}
	return searchKey(v, "workspace_id")
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

func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
