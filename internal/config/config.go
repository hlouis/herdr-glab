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

// glabHost returns the only host glab is configured for. glab's global `host`
// defaults to gitlab.com, so the hosts map is the reliable signal.
func glabHost() (string, error) {
	path := glabConfigPath()
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read glab config %s: %w; set host in config.toml", path, err)
	}
	var c struct {
		Hosts map[string]any `yaml:"hosts"`
	}
	if err := yaml.Unmarshal(data, &c); err != nil {
		return "", fmt.Errorf("parse glab config %s: %w; set host in config.toml", path, err)
	}
	hosts := slices.Sorted(maps.Keys(c.Hosts))
	if len(hosts) != 1 {
		return "", fmt.Errorf("glab config has hosts %v; set host in config.toml", hosts)
	}
	return hosts[0], nil
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
