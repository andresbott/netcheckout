package threewayrsync

import (
	"errors"
	"strconv"
	"strings"
)

// SSH describes reaching an endpoint over ssh. A zero Port and empty IdentityFile mean
// "use the ssh default".
type SSH struct {
	User         string
	Host         string
	Port         int
	IdentityFile string
}

// Endpoint is one side of a sync: an absolute path, optionally reached over ssh. A nil
// SSH means a local filesystem path (for example a mounted share).
type Endpoint struct {
	Path string
	SSH  *SSH
}

// render returns the rsync-syntax location: a plain path locally, or "[user@]host:path"
// for an ssh target.
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

// withTrailingSlash ensures the location ends in exactly one "/" so rsync copies the
// contents of the source rather than nesting a copy.
func withTrailingSlash(loc string) string {
	return strings.TrimRight(loc, "/") + "/"
}

// sshRsh builds the value for rsync's --rsh flag when a non-default port or identity file
// is set. It returns "" when ssh defaults suffice.
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

// validate checks a local/remote endpoint pair before any rsync invocation. rsync cannot
// transfer between two remote hosts, so at most one endpoint may be ssh.
func validate(local, remote Endpoint) error {
	if strings.TrimSpace(local.Path) == "" {
		return errors.New("local path is required")
	}
	if strings.TrimSpace(remote.Path) == "" {
		return errors.New("remote path is required")
	}
	if local.SSH != nil && remote.SSH != nil {
		return errors.New("at most one endpoint may be an ssh target")
	}
	if local.SSH != nil && strings.TrimSpace(local.SSH.Host) == "" {
		return errors.New("ssh host is required")
	}
	if remote.SSH != nil && strings.TrimSpace(remote.SSH.Host) == "" {
		return errors.New("ssh host is required")
	}
	return nil
}
