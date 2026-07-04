package rsync

import (
	"reflect"
	"strings"
	"testing"
)

func TestParseItemizeClassifiesChanges(t *testing.T) {
	out := strings.Join([]string{
		"sending incremental file list",
		".d..t...... ./",
		">f+++++++++ new.txt",
		">f.st...... changed.txt",
		"cd+++++++++ newdir/",
		".d..t...... existingdir/",
		"*deleting   gone.txt",
		"",
	}, "\n")
	got := parseItemize(out)
	want := []Change{
		{Path: "new.txt", Type: Created},
		{Path: "changed.txt", Type: Modified},
		{Path: "newdir/", Type: Created},
		{Path: "gone.txt", Type: Deleted},
	}
	if !reflect.DeepEqual(got.Changes, want) {
		t.Errorf("Changes = %#v, want %#v", got.Changes, want)
	}
	if got.InSync {
		t.Error("InSync = true, want false")
	}
}

func TestParseItemizeEmptyIsInSync(t *testing.T) {
	got := parseItemize("sending incremental file list\n\n")
	if !got.InSync || len(got.Changes) != 0 {
		t.Errorf("want in-sync empty, got %#v", got)
	}
}

func TestParseItemizePushUsesLeftAngle(t *testing.T) {
	got := parseItemize("<f+++++++++ pushed.txt\n")
	want := []Change{{Path: "pushed.txt", Type: Created}}
	if !reflect.DeepEqual(got.Changes, want) {
		t.Errorf("Changes = %#v, want %#v", got.Changes, want)
	}
}
