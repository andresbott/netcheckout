package rsync

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
)

// validate checks a Job before any rsync invocation.
func (j Job) validate() error {
	switch j.Direction {
	case Pull, Push:
	default:
		return fmt.Errorf("invalid direction %d", j.Direction)
	}
	if strings.TrimSpace(j.Local.Path) == "" {
		return errors.New("local path is required")
	}
	if strings.TrimSpace(j.Remote.Path) == "" {
		return errors.New("remote path is required")
	}
	if j.Local.SSH != nil {
		return errors.New("local endpoint must not be an ssh target")
	}
	if j.Remote.SSH != nil && strings.TrimSpace(j.Remote.SSH.Host) == "" {
		return errors.New("ssh host is required")
	}
	return nil
}

// render returns the rsync-syntax location for an endpoint: a plain path locally,
// or "[user@]host:path" for an ssh target.
func (e Endpoint) render() string {
	if e.SSH == nil {
		return e.Path
	}
	host := e.SSH.Host
	if e.SSH.User != "" {
		host = e.SSH.User + "@" + host
	}
	return host + ":" + e.Path
}

// withTrailingSlash ensures the location ends in exactly one "/" so rsync copies
// the contents of the source into the destination rather than nesting a copy.
func withTrailingSlash(loc string) string {
	return strings.TrimRight(loc, "/") + "/"
}

// sshRsh builds the value for rsync's --rsh flag when a non-default port or
// identity file is set. It returns "" when ssh defaults suffice.
func sshRsh(s *SSH) string {
	var parts []string
	if s.Port != 0 {
		parts = append(parts, "-p", strconv.Itoa(s.Port))
	}
	if strings.TrimSpace(s.IdentityFile) != "" {
		parts = append(parts, "-i", s.IdentityFile)
	}
	if len(parts) == 0 {
		return ""
	}
	return "ssh " + strings.Join(parts, " ")
}

// buildArgs assembles the rsync argument list for a job. --itemize-changes is
// always included so output can be parsed; --dry-run is added only for a diff.
func buildArgs(j Job, dryRun bool) ([]string, error) {
	if err := j.validate(); err != nil {
		return nil, err
	}
	args := []string{"--recursive", "--links", "--times", "--itemize-changes"}
	if dryRun {
		args = append(args, "--dry-run")
	}
	if j.Options.PreservePerms {
		args = append(args, "--perms")
	}
	if j.Options.PreserveOwner {
		args = append(args, "--owner")
	}
	if j.Options.PreserveGroup {
		args = append(args, "--group")
	}
	if j.Options.Checksum {
		args = append(args, "--checksum")
	}
	if j.Options.Delete {
		args = append(args, "--delete")
	}
	for _, e := range j.Options.Exclude {
		args = append(args, "--exclude="+e)
	}

	if j.Remote.SSH != nil {
		if rsh := sshRsh(j.Remote.SSH); rsh != "" {
			args = append(args, "--rsh="+rsh)
		}
	}

	src, dst := j.Remote, j.Local // Pull: Remote → Local
	if j.Direction == Push {
		src, dst = j.Local, j.Remote
	}
	args = append(args, withTrailingSlash(src.render()), dst.render())
	return args, nil
}
