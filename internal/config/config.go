// Package config loads $HERDR_PLUGIN_CONFIG_DIR/config.toml. See doc/design.md §10.
package config

import (
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"time"

	"github.com/BurntSushi/toml"
	"gopkg.in/yaml.v3"
)

const (
	defaultFetchInterval = 5 * time.Minute
	minFetchInterval     = time.Minute
	defaultTokenName     = "mr"
)

type Config struct {
	Host          string
	FetchInterval time.Duration
	GlabPath      string
	TuicrPath     string
	TokenName     string
}

type fileConfig struct {
	Host          string `toml:"host"`
	FetchInterval string `toml:"fetch_interval"`
	GlabPath      string `toml:"glab_path"`
	TuicrPath     string `toml:"tuicr_path"`
	TokenName     string `toml:"token_name"`
}

// Load reads config.toml from dir; a missing file means all defaults.
func Load(dir string) (Config, error) {
	var f fileConfig
	path := filepath.Join(dir, "config.toml")
	if _, err := toml.DecodeFile(path, &f); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return Config{}, fmt.Errorf("read %s: %w", path, err)
	}

	cfg := Config{
		Host:          f.Host,
		FetchInterval: defaultFetchInterval,
		GlabPath:      resolveBinary(f.GlabPath, "glab"),
		TuicrPath:     resolveBinary(f.TuicrPath, "tuicr"),
		TokenName:     defaultTokenName,
	}
	if f.TokenName != "" {
		cfg.TokenName = f.TokenName
	}
	if f.FetchInterval != "" {
		d, err := time.ParseDuration(f.FetchInterval)
		if err != nil {
			return Config{}, fmt.Errorf("fetch_interval %q: %w", f.FetchInterval, err)
		}
		cfg.FetchInterval = max(d, minFetchInterval)
	}
	if cfg.Host == "" {
		host, err := glabHost()
		if err != nil {
			return Config{}, err
		}
		cfg.Host = host
	}
	return cfg, nil
}

// glabHost reads the GitLab host out of glab's own configuration.
func glabHost() (string, error) {
	path := glabConfigPath()
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read glab config %s: %w; set host in config.toml", path, err)
	}
	host, err := resolveHost(data)
	if err != nil {
		return "", fmt.Errorf("%w (%s); set host in config.toml", err, path)
	}
	return host, nil
}

// resolveHost picks the host glab is logged in to when there is only one, which
// is the self-hosted case where glab's global default is still gitlab.com, and
// otherwise falls back to that global default.
func resolveHost(data []byte) (string, error) {
	var c struct {
		Host  string         `yaml:"host"`
		Hosts map[string]any `yaml:"hosts"`
	}
	if err := yaml.Unmarshal(data, &c); err != nil {
		return "", fmt.Errorf("parse glab config: %w", err)
	}

	hosts := slices.Sorted(maps.Keys(c.Hosts))
	switch {
	case len(hosts) == 1:
		return hosts[0], nil
	case c.Host != "":
		return c.Host, nil
	case len(hosts) == 0:
		return "", errors.New("glab is not logged in to any host")
	default:
		return "", fmt.Errorf("glab has several hosts %v and no default", hosts)
	}
}

func glabConfigPath() string {
	if dir := os.Getenv("GLAB_CONFIG_DIR"); dir != "" {
		return filepath.Join(dir, "config.yml")
	}
	if dir := os.Getenv("XDG_CONFIG_HOME"); dir != "" {
		return filepath.Join(dir, "glab-cli", "config.yml")
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "glab-cli", "config.yml")
}

// resolveBinary finds a tool even when herdr runs hooks with a minimal PATH.
func resolveBinary(configured, name string) string {
	if configured != "" {
		return configured
	}
	if path, err := exec.LookPath(name); err == nil {
		return path
	}
	for _, dir := range []string{"/opt/homebrew/bin", "/usr/local/bin"} {
		path := filepath.Join(dir, name)
		if info, err := os.Stat(path); err == nil && !info.IsDir() {
			return path
		}
	}
	return name
}
