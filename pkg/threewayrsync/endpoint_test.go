package threewayrsync

import "testing"

func TestEndpointRender(t *testing.T) {
	cases := []struct {
		name string
		ep   Endpoint
		want string
	}{
		{"local", Endpoint{Path: "/data"}, "/data"},
		{"ssh host only", Endpoint{Path: "/data", SSH: &SSH{Host: "nas"}}, "nas:/data"},
		{"ssh user host", Endpoint{Path: "/data", SSH: &SSH{User: "a", Host: "nas"}}, "a@nas:/data"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := c.ep.render(); got != c.want {
				t.Errorf("render = %q, want %q", got, c.want)
			}
		})
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

func TestSSHRsh(t *testing.T) {
	if got := sshRsh(&SSH{Host: "h"}); got != "" {
		t.Errorf("defaults should yield no rsh, got %q", got)
	}
	if got := sshRsh(&SSH{Host: "h", Port: 2222, IdentityFile: "/k"}); got != "ssh -p 2222 -i /k" {
		t.Errorf("rsh = %q", got)
	}
}

func TestValidate(t *testing.T) {
	local := Endpoint{Path: "/l"}
	remote := Endpoint{Path: "/r"}
	cases := map[string]struct {
		local, remote Endpoint
		wantErr       bool
	}{
		"both local":    {local, remote, false},
		"remote ssh ok": {local, Endpoint{Path: "/r", SSH: &SSH{Host: "h"}}, false},
		"local ssh ok":  {Endpoint{Path: "/l", SSH: &SSH{Host: "h"}}, remote, false},
		"both ssh":      {Endpoint{Path: "/l", SSH: &SSH{Host: "h"}}, Endpoint{Path: "/r", SSH: &SSH{Host: "h"}}, true},
		"empty local":   {Endpoint{Path: ""}, remote, true},
		"empty remote":  {local, Endpoint{Path: ""}, true},
		"ssh no host":   {local, Endpoint{Path: "/r", SSH: &SSH{Host: ""}}, true},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			err := validate(c.local, c.remote)
			if (err != nil) != c.wantErr {
				t.Errorf("validate err = %v, wantErr %v", err, c.wantErr)
			}
		})
	}
}
