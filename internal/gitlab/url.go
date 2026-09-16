package gitlab

import (
	"regexp"
	"strconv"
	"strings"
)

// GitLab groups nest, so the project path is everything between the host and
// the `/-/` separator.
var mrURL = regexp.MustCompile(`^https?://([^/]+)/(.+?)/-/merge_requests/(\d+)(?:[/#?].*)?$`)

// ParseMRURL splits a merge request URL into host, project full path and iid.
func ParseMRURL(raw string) (host, project string, iid int, ok bool) {
	match := mrURL.FindStringSubmatch(strings.TrimSpace(raw))
	if match == nil {
		return "", "", 0, false
	}
	iid, err := strconv.Atoi(match[3])
	if err != nil {
		return "", "", 0, false
	}
	return strings.ToLower(match[1]), match[2], iid, true
}
