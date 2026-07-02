package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// Profile is a named pair of roots: one on fast local disk, one on the network share.
type Profile struct {
	LocalRoot  string `yaml:"local_root"`
	RemoteRoot string `yaml:"remote_root"`
}

// Config is the on-disk configuration: an identity string and named profiles.
type Config struct {
	Identity string             `yaml:"identity,omitempty"`
	Profiles map[string]Profile `yaml:"profiles"`
}

// Load reads the YAML config at path. A missing file yields an empty config.
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path) //nolint:gosec // G304: config path is user-supplied via --config/$NETCHECKOUT_CONFIG by design; no trust boundary is crossed.
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return &Config{Profiles: map[string]Profile{}}, nil
		}
		return nil, err
	}
	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse config %s: %w", path, err)
	}
	if cfg.Profiles == nil {
		cfg.Profiles = map[string]Profile{}
	}
	return &cfg, nil
}

// Save writes cfg to path atomically (temp file + rename). Parent directories are
// created with mode 0700 and the resulting file has mode 0600.
func Save(path string, cfg *Config) error {
	if cfg.Profiles == nil {
		cfg.Profiles = map[string]Profile{}
	}
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return err
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".netcheckout-*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer func() { _ = os.Remove(tmpName) }() // no-op once renamed
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpName, path)
}

// DefaultPath returns the config file location: $NETCHECKOUT_CONFIG if set,
// otherwise the OS config directory + netcheckout/config.yaml.
func DefaultPath() (string, error) {
	if p := os.Getenv("NETCHECKOUT_CONFIG"); p != "" {
		return p, nil
	}
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "netcheckout", "config.yaml"), nil
}

// ExpandRoot expands environment variables and a leading ~ in a root path.
func ExpandRoot(root string) string {
	expanded := os.ExpandEnv(root)
	if expanded == "~" || strings.HasPrefix(expanded, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			expanded = filepath.Join(home, strings.TrimPrefix(expanded, "~"))
		}
	}
	return expanded
}

// ValidateName reports whether a profile name is usable.
func ValidateName(name string) error {
	if strings.TrimSpace(name) == "" {
		return errors.New("profile name is required")
	}
	return nil
}

// ValidateRoot reports whether a root path is usable: non-empty and absolute
// once ~ and environment variables are expanded.
func ValidateRoot(root string) error {
	if strings.TrimSpace(root) == "" {
		return errors.New("root path is required")
	}
	if !filepath.IsAbs(ExpandRoot(root)) {
		return errors.New("root path must be absolute")
	}
	return nil
}
