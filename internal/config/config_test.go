package config

import "testing"

func TestResolveHost(t *testing.T) {
	tests := []struct {
		name string
		yaml string
		want string
		fail bool
	}{
		{
			// The self-hosted case: glab keeps gitlab.com as its global default
			// even when the only account is somewhere else.
			name: "one host wins over the global default",
			yaml: "host: gitlab.com\nhosts:\n    gitlab.hlouis.com:\n        token: x\n",
			want: "gitlab.hlouis.com",
		},
		{
			name: "several hosts fall back to the global default",
			yaml: "host: gitlab.example.com\nhosts:\n    gitlab.com:\n        token: x\n    gitlab.example.com:\n        token: y\n",
			want: "gitlab.example.com",
		},
		{
			name: "no accounts but a default",
			yaml: "host: gitlab.com\n",
			want: "gitlab.com",
		},
		{
			name: "several hosts and no default",
			yaml: "hosts:\n    gitlab.com:\n        token: x\n    gitlab.example.com:\n        token: y\n",
			fail: true,
		},
		{
			name: "nothing configured",
			yaml: "git_protocol: ssh\n",
			fail: true,
		},
	}
	for _, tt := range tests {
		got, err := resolveHost([]byte(tt.yaml))
		if tt.fail {
			if err == nil {
				t.Errorf("%s: resolveHost() = %q, want an error", tt.name, got)
			}
			continue
		}
		if err != nil {
			t.Errorf("%s: resolveHost() failed: %v", tt.name, err)
		} else if got != tt.want {
			t.Errorf("%s: resolveHost() = %q, want %q", tt.name, got, tt.want)
		}
	}
}
