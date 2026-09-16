package gitlab

import "testing"

func TestParseMRURL(t *testing.T) {
	tests := []struct {
		raw     string
		host    string
		project string
		iid     int
		ok      bool
	}{
		{"https://gitlab.hlouis.com/louis/foodlib/-/merge_requests/1", "gitlab.hlouis.com", "louis/foodlib", 1, true},
		{"https://GitLab.hlouis.com/project-immt/atlas/atlas-server/-/merge_requests/412/diffs", "gitlab.hlouis.com", "project-immt/atlas/atlas-server", 412, true},
		{"https://gitlab.com/g/r/-/merge_requests/7#note_1", "gitlab.com", "g/r", 7, true},
		{"https://gitlab.hlouis.com/louis/foodlib/-/issues/3", "", "", 0, false},
		{"https://gitlab.hlouis.com/louis/foodlib", "", "", 0, false},
		{"not a url", "", "", 0, false},
	}
	for _, tt := range tests {
		host, project, iid, ok := ParseMRURL(tt.raw)
		if host != tt.host || project != tt.project || iid != tt.iid || ok != tt.ok {
			t.Errorf("ParseMRURL(%q) = %q, %q, %d, %v; want %q, %q, %d, %v",
				tt.raw, host, project, iid, ok, tt.host, tt.project, tt.iid, tt.ok)
		}
	}
}
