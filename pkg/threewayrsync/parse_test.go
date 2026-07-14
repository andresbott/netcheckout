package threewayrsync

import (
	"testing"
	"time"
)

func TestParseListOutput(t *testing.T) {
	out := "sending incremental file list\n" +
		">f+++++++++ 1 2026/07/14-09:15:00 a.txt\n" +
		">f+++++++++ 2 2026/07/14-09:15:01 sub/b.txt\n" +
		"cd+++++++++ 4096 2026/07/14-09:15:00 sub\n" +
		".d..t...... 4096 2026/07/14-09:15:00 ./\n" +
		"\n"
	m, err := parseListOutput(out)
	if err != nil {
		t.Fatal(err)
	}
	if len(m) != 2 {
		t.Fatalf("want 2 regular files, got %d: %+v", len(m), m)
	}
	if m["a.txt"].Size != 1 {
		t.Errorf("a.txt size = %d, want 1", m["a.txt"].Size)
	}
	want := time.Date(2026, 7, 14, 9, 15, 1, 0, time.Local)
	if !m["sub/b.txt"].ModTime.Equal(want) {
		t.Errorf("sub/b.txt mtime = %v, want %v", m["sub/b.txt"].ModTime, want)
	}
	if _, ok := m["sub"]; ok {
		t.Error("directory must be excluded from the manifest")
	}
}

func TestParseListLineSkipsChatterAndDirs(t *testing.T) {
	for _, line := range []string{
		"sending incremental file list",
		"",
		"cd+++++++++ 4096 2026/07/14-09:15:00 sub", // directory (2nd flag 'd')
		".d..t...... 4096 2026/07/14-09:15:00 ./",   // root
	} {
		if _, _, ok := parseListLine(line); ok {
			t.Errorf("line should be skipped: %q", line)
		}
	}
}

func TestItemizeWriterStreamsPaths(t *testing.T) {
	var got []string
	w := &itemizeWriter{onPath: func(p string) { got = append(got, p) }}
	// Split one line across two writes to prove partial input is buffered.
	chunks := []string{
		"sending incremental file list\n>f+++++++++ new.txt\n>f.st",
		"...... changed.txt\n",
	}
	for _, c := range chunks {
		if _, err := w.Write([]byte(c)); err != nil {
			t.Fatal(err)
		}
	}
	if len(got) != 2 || got[0] != "new.txt" || got[1] != "changed.txt" {
		t.Errorf("streamed paths = %v", got)
	}
}
