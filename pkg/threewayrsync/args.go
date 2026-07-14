package threewayrsync

import (
	"os"
	"strconv"
	"strings"
)

// buildListArgs assembles the rsync argument list for enumerating src via a dry-run
// itemize against an empty destination. --out-format prints "%i %l %M %n" (itemize flags,
// size, mtime, path) per entry. emptyDest must be an existing empty directory.
func buildListArgs(src Endpoint, emptyDest string, exclude []string) []string {
	args := []string{"--recursive", "--links", "--dry-run", "--itemize-changes", "--out-format=%i %l %M %n"}
	for _, ex := range exclude {
		args = append(args, "--exclude="+ex)
	}
	if src.SSH != nil {
		if rsh := sshRsh(src.SSH); rsh != "" {
			args = append(args, "--rsh="+rsh)
		}
	}
	args = append(args, withTrailingSlash(src.render()), withTrailingSlash(emptyDest))
	return args
}

// buildTransferArgs assembles the rsync argument list for a real transfer from src to dst.
// --partial lets a canceled transfer resume; --times equalizes mtime so a re-listing sees
// the two sides as equal. At most one of src/dst is ssh (validated earlier).
func buildTransferArgs(src, dst Endpoint, checksum bool, exclude []string) []string {
	args := []string{"--recursive", "--links", "--times", "--itemize-changes", "--partial"}
	if checksum {
		args = append(args, "--checksum")
	}
	for _, ex := range exclude {
		args = append(args, "--exclude="+ex)
	}
	if src.SSH != nil {
		if rsh := sshRsh(src.SSH); rsh != "" {
			args = append(args, "--rsh="+rsh)
		}
	}
	if dst.SSH != nil {
		if rsh := sshRsh(dst.SSH); rsh != "" {
			args = append(args, "--rsh="+rsh)
		}
	}
	args = append(args, withTrailingSlash(src.render()), dst.render())
	return args
}

// withFilesFrom splices "--files-from=<path>" in front of the two trailing positional
// paths so rsync parses it as an option, not a source.
func withFilesFrom(args []string, listPath string) []string {
	n := len(args)
	out := make([]string, 0, n+1)
	out = append(out, args[:n-2]...)
	out = append(out, "--files-from="+listPath)
	out = append(out, args[n-2:]...)
	return out
}

// writeFileList writes one relative path per line to a temp file and returns its name; the
// caller removes it.
func writeFileList(paths []string) (string, error) {
	f, err := os.CreateTemp("", "threewayrsync-files-*.txt")
	if err != nil {
		return "", err
	}
	name := f.Name()
	if _, err := f.WriteString(strings.Join(paths, "\n") + "\n"); err != nil {
		_ = f.Close()
		_ = os.Remove(name)
		return "", err
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(name)
		return "", err
	}
	return name, nil
}

// sshCmdArgs builds the ssh argument prefix "[-p port] [-i identity] [user@]host" used to
// run a remote command (a targeted delete).
func sshCmdArgs(s *SSH) []string {
	var args []string
	if s.Port != 0 {
		args = append(args, "-p", strconv.Itoa(s.Port))
	}
	if strings.TrimSpace(s.IdentityFile) != "" {
		args = append(args, "-i", s.IdentityFile)
	}
	host := s.Host
	if s.User != "" {
		host = s.User + "@" + host
	}
	return append(args, host)
}
