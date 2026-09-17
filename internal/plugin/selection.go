package plugin

import "path/filepath"

// SelectedMRPath hands a merge request from the panel to the threads pane, the
// same way ClickedURLPath does for a link click: herdr starts a pane without
// the invoking action's context.
func (e Env) SelectedMRPath() string {
	return filepath.Join(e.StateDir, "selected-mr.json")
}
