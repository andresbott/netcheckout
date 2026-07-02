package config_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/andresbott/netcheckout/internal/config"
)

func TestLoadMissingFileReturnsEmpty(t *testing.T) {
	cfg, err := config.Load(filepath.Join(t.TempDir(), "does-not-exist.yaml"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg == nil || cfg.Profiles == nil || len(cfg.Profiles) != 0 {
		t.Fatalf("want empty non-nil config, got %#v", cfg)
	}
}

func TestLoadParsesProfiles(t *testing.T) {
	p := filepath.Join(t.TempDir(), "config.yaml")
	data := "identity: me@host\nprofiles:\n  photos:\n    local_root: /home/me/pics\n    remote_root: /mnt/nas/pics\n"
	if err := os.WriteFile(p, []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.Load(p)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Identity != "me@host" {
		t.Errorf("identity = %q", cfg.Identity)
	}
	got := cfg.Profiles["photos"]
	if got.LocalRoot != "/home/me/pics" || got.RemoteRoot != "/mnt/nas/pics" {
		t.Errorf("photos = %#v", got)
	}
}

func TestLoadMalformedYAMLErrors(t *testing.T) {
	p := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(p, []byte("profiles: [unterminated\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := config.Load(p); err == nil {
		t.Fatal("expected parse error, got nil")
	}
}

func TestLoadValidNoProfilesKey(t *testing.T) {
	p := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(p, []byte("identity: me@host\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.Load(p)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Profiles == nil {
		t.Fatal("Profiles should be non-nil after loading YAML without a profiles key")
	}
	if len(cfg.Profiles) != 0 {
		t.Fatalf("Profiles should be empty, got %d entries", len(cfg.Profiles))
	}
}
