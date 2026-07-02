package config

import (
	"errors"
	"fmt"
	"os"

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
