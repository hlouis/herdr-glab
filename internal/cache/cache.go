// Package cache stores fetched GitLab data under $HERDR_PLUGIN_STATE_DIR. See doc/design.md §5.
package cache

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/hlouis/herdr-glab/internal/gitlab"
)

const Version = 1

type Cache struct {
	Version   int                   `json:"version"`
	Host      string                `json:"host"`
	Username  string                `json:"username"`
	FetchedAt time.Time             `json:"fetched_at"`
	Error     string                `json:"error"`
	Mine      []gitlab.MergeRequest `json:"mine"`
	BranchMRs []gitlab.MergeRequest `json:"branch_mrs"`
}

func Path(stateDir string) string {
	return filepath.Join(stateDir, "cache.json")
}

// Load reads the cache; a missing or incompatible file yields an empty cache.
func Load(path string) (Cache, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return Cache{}, nil
	}
	if err != nil {
		return Cache{}, err
	}
	var c Cache
	if err := json.Unmarshal(data, &c); err != nil || c.Version != Version {
		return Cache{}, nil
	}
	return c, nil
}

// Save writes through a temp file and rename so readers never see a partial file.
func Save(path string, c Cache) error {
	c.Version = Version
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".cache-*.json")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}

// FindBySourceBranch finds an MR whose source branch lives in project.
func (c Cache) FindBySourceBranch(project, branch string) (gitlab.MergeRequest, bool) {
	return c.find(func(m gitlab.MergeRequest) bool {
		return m.SourceBranch == branch && strings.EqualFold(m.SourceProject, project)
	})
}

// FindByIID finds an MR targeting project by iid.
func (c Cache) FindByIID(project string, iid int) (gitlab.MergeRequest, bool) {
	return c.find(func(m gitlab.MergeRequest) bool {
		return m.IID == iid && strings.EqualFold(m.Project, project)
	})
}

func (c Cache) find(match func(gitlab.MergeRequest) bool) (gitlab.MergeRequest, bool) {
	for _, list := range [][]gitlab.MergeRequest{c.Mine, c.BranchMRs} {
		for _, m := range list {
			if match(m) {
				return m, true
			}
		}
	}
	return gitlab.MergeRequest{}, false
}
