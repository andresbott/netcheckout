package threewayrsync

import (
	"testing"
	"time"
)

func TestFileStateEqual(t *testing.T) {
	t0 := time.Unix(100, 0)
	a := FileState{Size: 5, ModTime: t0}
	cases := []struct {
		name string
		b    FileState
		want bool
	}{
		{"identical", FileState{Size: 5, ModTime: t0}, true},
		{"size differs", FileState{Size: 6, ModTime: t0}, false},
		{"mtime differs", FileState{Size: 5, ModTime: time.Unix(101, 0)}, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := a.Equal(c.b); got != c.want {
				t.Errorf("Equal = %v, want %v", got, c.want)
			}
		})
	}
}
