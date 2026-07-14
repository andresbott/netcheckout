package threewayrsync

import (
	"bufio"
	"bytes"
	"strconv"
	"strings"
	"time"
)

// listMtimeLayout matches rsync's %M escape: "YYYY/MM/DD-HH:MM:SS" in local time. If the
// Task 1 fixture shows a different form, update this and parseListLine together.
const listMtimeLayout = "2006/01/02-15:04:05"

// parseListOutput turns "%i %l %M %n" list output into a Manifest of regular files.
func parseListOutput(out string) (Manifest, error) {
	m := Manifest{}
	sc := bufio.NewScanner(strings.NewReader(out))
	sc.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	for sc.Scan() {
		if fs, path, ok := parseListLine(sc.Text()); ok {
			m[path] = fs
		}
	}
	return m, sc.Err()
}

// parseListLine parses one "%i %l %M %n" line, keeping regular files only. It skips
// directories, symlinks, the "./" root, and any non-itemize chatter (blank lines, the
// "sending incremental file list" banner). ok is false for a skipped line.
func parseListLine(line string) (FileState, string, bool) {
	parts := strings.SplitN(line, " ", 4)
	if len(parts) < 4 {
		return FileState{}, "", false
	}
	flags, sizeStr, mtimeStr, path := parts[0], parts[1], parts[2], parts[3]
	if len(flags) != 11 || flags[1] != 'f' { // regular files only (2nd flag char)
		return FileState{}, "", false
	}
	if path == "" || path == "./" {
		return FileState{}, "", false
	}
	size, err := strconv.ParseInt(strings.ReplaceAll(sizeStr, ",", ""), 10, 64)
	if err != nil {
		return FileState{}, "", false
	}
	mtime, err := time.ParseInLocation(listMtimeLayout, mtimeStr, time.Local)
	if err != nil {
		return FileState{}, "", false
	}
	return FileState{Size: size, ModTime: mtime}, path, true
}

// itemizeWriter extracts the path from each rsync --itemize-changes line as it streams and
// invokes onPath. Writes may split a line, so partial input is buffered until its newline.
type itemizeWriter struct {
	onPath func(string)
	buf    []byte
}

func (w *itemizeWriter) Write(p []byte) (int, error) {
	w.buf = append(w.buf, p...)
	for {
		i := bytes.IndexByte(w.buf, '\n')
		if i < 0 {
			break
		}
		line := strings.TrimRight(string(w.buf[:i]), "\r")
		w.buf = w.buf[i+1:]
		if path, ok := itemizePath(line); ok {
			w.onPath(path)
		}
	}
	return len(p), nil
}

// itemizePath returns the path from a valid --itemize-changes line (11-char flag field
// then the path), skipping the "./" root and non-itemize chatter.
func itemizePath(line string) (string, bool) {
	const flagLen = 11
	if len(line) < flagLen+1 {
		return "", false
	}
	flags := line[:flagLen]
	if !strings.ContainsRune("<>ch.*", rune(flags[0])) {
		return "", false
	}
	path := strings.TrimSpace(line[flagLen:])
	if path == "" || path == "./" {
		return "", false
	}
	return path, true
}
