package repo

import "testing"

func TestParseRemote(t *testing.T) {
	tests := []struct {
		raw, host, project string
		ok                 bool
	}{
		{"git@gitlab.hlouis.com:project-immt/atlas/atlas-server.git", "gitlab.hlouis.com", "project-immt/atlas/atlas-server", true},
		{"https://gitlab.hlouis.com/louis/foodlib.git", "gitlab.hlouis.com", "louis/foodlib", true},
		{"https://oauth2:token@GitLab.hlouis.com:8443/Louis/FoodLib/", "gitlab.hlouis.com", "louis/foodlib", true},
		{"ssh://git@gitlab.hlouis.com:2222/group/sub/repo.git", "gitlab.hlouis.com", "group/sub/repo", true},
		{"/local/path/repo", "", "", false},
		{"", "", "", false},
	}
	for _, tt := range tests {
		host, project, ok := ParseRemote(tt.raw)
		if host != tt.host || project != tt.project || ok != tt.ok {
			t.Errorf("ParseRemote(%q) = %q, %q, %v; want %q, %q, %v", tt.raw, host, project, ok, tt.host, tt.project, tt.ok)
		}
	}
}
