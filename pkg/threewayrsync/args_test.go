package threewayrsync

import (
	"os"
	"slices"
	"testing"
)

func TestBuildListArgsLocal(t *testing.T) {
	got := buildListArgs(Endpoint{Path: "/data"}, "/tmp/empty", nil)
	for _, want := range []string{"--recursive", "--dry-run", "--itemize-changes", "--out-format=%i %l %M %n"} {
		if !slices.Contains(got, want) {
			t.Errorf("list args missing %q: %v", want, got)
		}
	}
	if got[len(got)-2] != "/data/" || got[len(got)-1] != "/tmp/empty/" {
		t.Errorf("src/dst = %q %q", got[len(got)-2], got[len(got)-1])
	}
}

func TestBuildListArgsSSHAddsRsh(t *testing.T) {
	got := buildListArgs(Endpoint{Path: "/data", SSH: &SSH{Host: "nas", Port: 2222}}, "/tmp/empty", nil)
	if !slices.Contains(got, "--rsh=ssh -p 2222") {
		t.Errorf("missing rsh: %v", got)
	}
	if got[len(got)-2] != "nas:/data/" {
		t.Errorf("src = %q", got[len(got)-2])
	}
}

func TestBuildTransferArgsHasPartialAndFiles(t *testing.T) {
	got := buildTransferArgs(Endpoint{Path: "/src"}, Endpoint{Path: "/dst"}, false, nil)
	for _, want := range []string{"--recursive", "--links", "--times", "--itemize-changes", "--partial"} {
		if !slices.Contains(got, want) {
			t.Errorf("transfer args missing %q: %v", want, got)
		}
	}
	if slices.Contains(got, "--checksum") {
		t.Errorf("checksum should be off by default: %v", got)
	}
	if got[len(got)-2] != "/src/" || got[len(got)-1] != "/dst" {
		t.Errorf("src/dst = %q %q", got[len(got)-2], got[len(got)-1])
	}
}

func TestBuildTransferArgsChecksumAndExclude(t *testing.T) {
	got := buildTransferArgs(Endpoint{Path: "/src"}, Endpoint{Path: "/dst", SSH: &SSH{Host: "h", Port: 2222}}, true, []string{".git"})
	for _, want := range []string{"--checksum", "--exclude=.git", "--rsh=ssh -p 2222"} {
		if !slices.Contains(got, want) {
			t.Errorf("missing %q: %v", want, got)
		}
	}
	if got[len(got)-1] != "h:/dst" {
		t.Errorf("dst = %q", got[len(got)-1])
	}
}

func TestWithFilesFromInsertsBeforePaths(t *testing.T) {
	args := []string{"--recursive", "--partial", "/src/", "/dst"}
	got := withFilesFrom(args, "/tmp/list")
	if got[len(got)-3] != "--files-from=/tmp/list" {
		t.Fatalf("files-from not spliced before paths: %v", got)
	}
	if got[len(got)-2] != "/src/" || got[len(got)-1] != "/dst" {
		t.Fatalf("positional paths disturbed: %v", got)
	}
}

func TestWriteFileListRoundTrip(t *testing.T) {
	name, err := writeFileList([]string{"a.txt", "sub/b.txt"})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Remove(name) }()
	data, err := os.ReadFile(name)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "a.txt\nsub/b.txt\n" {
		t.Errorf("file list = %q", string(data))
	}
}

func TestSSHCmdArgs(t *testing.T) {
	got := sshCmdArgs(&SSH{User: "a", Host: "nas", Port: 2222, IdentityFile: "/k"})
	want := []string{"-p", "2222", "-i", "/k", "a@nas"}
	if !slices.Equal(got, want) {
		t.Errorf("sshCmdArgs = %v, want %v", got, want)
	}
	if got := sshCmdArgs(&SSH{Host: "nas"}); !slices.Equal(got, []string{"nas"}) {
		t.Errorf("defaults = %v, want [nas]", got)
	}
}
