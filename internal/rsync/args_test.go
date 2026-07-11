package rsync

import (
	"reflect"
	"slices"
	"strings"
	"testing"
)

func TestBuildArgsPullLocalDefaults(t *testing.T) {
	j := Job{
		Local:     Endpoint{Path: "/local/reports"},
		Remote:    Endpoint{Path: "/mnt/nas/reports"},
		Direction: Pull,
	}
	got, err := buildArgs(j, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := []string{
		"--recursive", "--links", "--times", "--itemize-changes",
		"/mnt/nas/reports/", "/local/reports",
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("buildArgs = %v, want %v", got, want)
	}
}

func TestBuildArgsDiffAddsDryRun(t *testing.T) {
	j := Job{Local: Endpoint{Path: "/l"}, Remote: Endpoint{Path: "/r"}, Direction: Pull}
	got, _ := buildArgs(j, true)
	if !slices.Contains(got, "--dry-run") {
		t.Errorf("diff args missing --dry-run: %v", got)
	}
}

func TestBuildArgsPushSwapsSourceAndDest(t *testing.T) {
	j := Job{Local: Endpoint{Path: "/local"}, Remote: Endpoint{Path: "/remote"}, Direction: Push}
	got, _ := buildArgs(j, false)
	src, dst := got[len(got)-2], got[len(got)-1]
	if src != "/local/" || dst != "/remote" {
		t.Errorf("push src,dst = %q,%q want /local/,/remote", src, dst)
	}
}

func TestBuildArgsOptionsMapToFlags(t *testing.T) {
	j := Job{
		Local: Endpoint{Path: "/l"}, Remote: Endpoint{Path: "/r"}, Direction: Push,
		Options: Options{Delete: true, PreservePerms: true, PreserveOwner: true, PreserveGroup: true, Checksum: true},
	}
	got, _ := buildArgs(j, false)
	for _, want := range []string{"--perms", "--owner", "--group", "--checksum", "--delete"} {
		if !slices.Contains(got, want) {
			t.Errorf("args missing %q: %v", want, got)
		}
	}
}

func TestBuildArgsZeroOptionsHaveNoExtraFlags(t *testing.T) {
	j := Job{Local: Endpoint{Path: "/l"}, Remote: Endpoint{Path: "/r"}, Direction: Push}
	got, _ := buildArgs(j, false)
	for _, absent := range []string{"--perms", "--owner", "--group", "--checksum", "--delete", "--archive"} {
		if slices.Contains(got, absent) {
			t.Errorf("args unexpectedly contains %q: %v", absent, got)
		}
	}
}

func TestBuildArgsSSHRemoteRendersHostAndRsh(t *testing.T) {
	j := Job{
		Local:     Endpoint{Path: "/local"},
		Remote:    Endpoint{Path: "/data", SSH: &SSH{User: "andres", Host: "nas", Port: 2222, IdentityFile: "/keys/id"}},
		Direction: Pull,
	}
	got, err := buildArgs(j, false)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(got, "--rsh=ssh -p 2222 -i /keys/id") {
		t.Errorf("missing rsh: %v", got)
	}
	if got[len(got)-2] != "andres@nas:/data/" {
		t.Errorf("src = %q want andres@nas:/data/", got[len(got)-2])
	}
}

func TestBuildArgsSSHDefaultPortNoRsh(t *testing.T) {
	j := Job{
		Local:     Endpoint{Path: "/local"},
		Remote:    Endpoint{Path: "/data", SSH: &SSH{Host: "nas"}},
		Direction: Push,
	}
	got, _ := buildArgs(j, false)
	for _, a := range got {
		if strings.HasPrefix(a, "--rsh=") {
			t.Errorf("unexpected rsh with default ssh: %v", got)
		}
	}
	if got[len(got)-1] != "nas:/data" {
		t.Errorf("dst = %q want nas:/data", got[len(got)-1])
	}
}

func TestBuildArgsValidation(t *testing.T) {
	tests := map[string]Job{
		"empty local path":  {Local: Endpoint{Path: ""}, Remote: Endpoint{Path: "/r"}, Direction: Pull},
		"empty remote path": {Local: Endpoint{Path: "/l"}, Remote: Endpoint{Path: ""}, Direction: Pull},
		"local is ssh":      {Local: Endpoint{Path: "/l", SSH: &SSH{Host: "h"}}, Remote: Endpoint{Path: "/r"}, Direction: Pull},
		"ssh without host":  {Local: Endpoint{Path: "/l"}, Remote: Endpoint{Path: "/r", SSH: &SSH{Host: ""}}, Direction: Pull},
		"bad direction":     {Local: Endpoint{Path: "/l"}, Remote: Endpoint{Path: "/r"}, Direction: Direction(99)},
	}
	for name, j := range tests {
		t.Run(name, func(t *testing.T) {
			if _, err := buildArgs(j, false); err == nil {
				t.Errorf("expected validation error for %s", name)
			}
		})
	}
}

func TestBuildArgsExcludeAddsFlag(t *testing.T) {
	j := Job{Local: Endpoint{Path: "/l"}, Remote: Endpoint{Path: "/r"}, Direction: Pull,
		Options: Options{Exclude: []string{".netcheckout.json"}}}
	got, _ := buildArgs(j, false)
	if !slices.Contains(got, "--exclude=.netcheckout.json") {
		t.Errorf("args missing --exclude: %v", got)
	}
}

func TestWithFilesFromInsertsBeforePaths(t *testing.T) {
	args := []string{"--recursive", "--links", "--times", "--itemize-changes", "/src/", "/dst"}
	got := withFilesFrom(args, "/tmp/list")
	// --files-from must come before the two trailing positional paths.
	if got[len(got)-3] != "--files-from=/tmp/list" {
		t.Fatalf("files-from not spliced before paths: %v", got)
	}
	if got[len(got)-2] != "/src/" || got[len(got)-1] != "/dst" {
		t.Fatalf("positional paths disturbed: %v", got)
	}
}

func TestWithTrailingSlashIdempotent(t *testing.T) {
	if got := withTrailingSlash("/a/b"); got != "/a/b/" {
		t.Errorf("got %q", got)
	}
	if got := withTrailingSlash("/a/b/"); got != "/a/b/" {
		t.Errorf("got %q", got)
	}
}
