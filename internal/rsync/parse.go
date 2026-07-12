package rsync

import (
	"bufio"
	"strings"
)

const itemizeFlagLen = 11

// parseItemize turns rsync --itemize-changes output into a structured Diff. It
// keeps file changes and directory creations/deletions, and ignores the "./" root
// entry, attribute-only directory churn, and any non-itemize chatter.
func parseItemize(out string) Diff {
	var changes []Change
	sc := bufio.NewScanner(strings.NewReader(out))
	for sc.Scan() {
		if c, ok := classifyItemizeLine(sc.Text()); ok {
			changes = append(changes, c)
		}
	}
	return Diff{Changes: changes, InSync: len(changes) == 0}
}

// classifyItemizeLine turns a single rsync --itemize-changes line into a Change,
// reporting whether the line was a change worth keeping. It ignores the "./" root
// entry, attribute-only directory churn, and any non-itemize chatter. It is the
// single per-line classifier shared by the batch parser (parseItemize) and the
// live streaming writer (itemizeWriter).
func classifyItemizeLine(line string) (Change, bool) {
	if strings.HasPrefix(line, "*deleting") {
		if path := strings.TrimSpace(strings.TrimPrefix(line, "*deleting")); path != "" {
			return Change{Path: path, Type: Deleted}, true
		}
		return Change{}, false
	}
	flags, path, ok := splitItemizeLine(line)
	if !ok || path == "" || path == "./" {
		return Change{}, false
	}
	attrs := flags[2:]
	if isDir(flags) && !allPlus(attrs) {
		return Change{}, false // attribute-only directory change is noise
	}
	if allPlus(attrs) {
		return Change{Path: path, Type: Created}, true
	}
	return Change{Path: path, Type: Modified}, true
}

// splitItemizeLine splits a line into its 11-char itemize flag field and path,
// reporting whether the line is a valid itemize line.
func splitItemizeLine(line string) (flags, path string, ok bool) {
	if len(line) < itemizeFlagLen+1 {
		return "", "", false
	}
	flags = line[:itemizeFlagLen]
	if !isItemizeFlags(flags) {
		return "", "", false
	}
	return flags, strings.TrimSpace(line[itemizeFlagLen:]), true
}

// isItemizeFlags reports whether f is a plausible 11-char itemize flag field.
func isItemizeFlags(f string) bool {
	if len(f) != itemizeFlagLen {
		return false
	}
	if !strings.ContainsRune("<>ch.", rune(f[0])) {
		return false
	}
	if !strings.ContainsRune("fdLDS", rune(f[1])) {
		return false
	}
	for _, c := range f[2:] {
		switch {
		case c == '.' || c == '+' || c == ' ':
		case c >= 'a' && c <= 'z':
		case c >= 'A' && c <= 'Z':
		default:
			return false
		}
	}
	return true
}

func isDir(flags string) bool { return len(flags) > 1 && flags[1] == 'd' }

// allPlus reports whether the 9-char attribute field is all "+", marking a newly
// created item.
func allPlus(attrs string) bool {
	if attrs == "" {
		return false
	}
	for _, c := range attrs {
		if c != '+' {
			return false
		}
	}
	return true
}
