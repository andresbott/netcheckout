package lifecycle

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/andresbott/netcheckout/internal/config"
	"github.com/andresbott/netcheckout/internal/ident"
	"github.com/andresbott/netcheckout/internal/marker"
)

func TestSyncRefusesUnlistedLocalContent(t *testing.T) {
	local, remote := t.TempDir(), t.TempDir()
	// A marker so the guard is reached only if it runs BEFORE the lock check would
	// otherwise pass; the guard must fire regardless.
	id := ident.Ident{By: "me@host", Host: "host"}
	m := &marker.Marker{CheckedOutBy: id.By, Host: id.Host, Profile: "p"}
	if err := marker.Write(remote, m); err != nil {
		t.Fatal(err)
	}
	// Unlisted local content: subpaths=[a] but a file lives at the root.
	if err := os.WriteFile(filepath.Join(local, "top.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	p := config.Profile{LocalRoot: local, RemoteRoot: remote, Subpaths: []string{"a"}}
	r := Runner{Syncer: &fakeSyncer{}, ToolVersion: "test"}

	_, err := r.Sync(context.Background(), "p", p, id, "", Options{})
	if err == nil || !strings.Contains(err.Error(), "top.txt") {
		t.Fatalf("expected error naming top.txt, got %v", err)
	}
}
