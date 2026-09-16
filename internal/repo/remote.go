// Package repo maps herdr workspaces to GitLab projects and merge requests. See doc/design.md §6.
package repo

import (
	"net/url"
	"strings"
)

// ParseRemote splits a git remote URL into a lowercase host and project path.
// It accepts https://, ssh:// and SCP-style user@host:path forms.
func ParseRemote(raw string) (host, project string, ok bool) {
	s := strings.TrimSpace(raw)
	if strings.Contains(s, "://") {
		u, err := url.Parse(s)
		if err != nil {
			return "", "", false
		}
		host, project = u.Hostname(), u.Path
	} else {
		hostPart, pathPart, found := strings.Cut(s, ":")
		if !found {
			return "", "", false
		}
		if i := strings.LastIndex(hostPart, "@"); i >= 0 {
			hostPart = hostPart[i+1:]
		}
		host, project = hostPart, pathPart
	}
	project = strings.Trim(strings.TrimSuffix(strings.Trim(project, "/"), ".git"), "/")
	if host == "" || project == "" {
		return "", "", false
	}
	return strings.ToLower(host), strings.ToLower(project), true
}
