package discovery

import "testing"

func TestPortFromArgs(t *testing.T) {
	cases := []struct {
		args []string
		want int
	}{
		{[]string{"tilt", "up"}, 0},
		{[]string{"tilt", "up", "--port", "10351"}, 10351},
		{[]string{"tilt", "up", "--port=10352"}, 10352},
		{[]string{"tilt", "up", "-port", "10353"}, 10353},
		{[]string{"tilt", "up", "--port", "notanumber"}, 0},
	}
	for _, c := range cases {
		if got := portFromArgs(c.args); got != c.want {
			t.Errorf("portFromArgs(%v) = %d, want %d", c.args, got, c.want)
		}
	}
}

func TestIsTiltUp(t *testing.T) {
	yes := [][]string{
		{"tilt", "up"},
		{"/usr/local/bin/tilt", "up", "--port", "10350"},
		{"tilt", "up", "--stream"},
	}
	no := [][]string{
		{"tilt"},               // no subcommand
		{"tilt", "down"},       // wrong subcommand
		{"vim", "Tiltfile"},    // not tilt
		{"grep", "tilt", "up"}, // base is grep, not tilt
	}
	for _, a := range yes {
		if !isTiltUp(a) {
			t.Errorf("isTiltUp(%v) = false, want true", a)
		}
	}
	for _, a := range no {
		if isTiltUp(a) {
			t.Errorf("isTiltUp(%v) = true, want false", a)
		}
	}
}

func TestLabelFromPath(t *testing.T) {
	cases := map[string]string{
		"/home/me/myapp/k8s/overlays/local": "myapp",
		"/home/me/myapp":                    "myapp",
		"/srv/projects/web/deploy/dev":      "web",
	}
	for in, want := range cases {
		if got := labelFromPath(in); got != want {
			t.Errorf("labelFromPath(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestDisambiguate(t *testing.T) {
	insts := []Instance{
		{Port: 10350, Label: "app"},
		{Port: 10360, Label: "app"},
		{Port: 10370, Label: "solo"},
		{Port: 10380, Label: ""},
	}
	disambiguate(insts)
	if insts[0].Label != "app:10350" || insts[1].Label != "app:10360" {
		t.Errorf("shared labels not disambiguated: %q, %q", insts[0].Label, insts[1].Label)
	}
	if insts[2].Label != "solo" {
		t.Errorf("unique label changed: %q", insts[2].Label)
	}
	if insts[3].Label != "10380" {
		t.Errorf("empty label not filled with port: %q", insts[3].Label)
	}
}

// TestParseTiltProcesses exercises the macOS `ps`-output parser with injected cwd
// and env lookups, so the mac path is covered on any OS.
func TestParseTiltProcesses(t *testing.T) {
	ps := "  101 /usr/local/bin/tilt up --port 10351\n" +
		"  202 tilt up\n" + // no port flag -> env lookup
		"  303 vim Tiltfile\n" + // not tilt up
		"  404 /bin/tilt down\n" + // wrong subcommand
		"\n" // blank line tolerated
	cwd := func(pid int) string {
		return map[int]string{101: "/home/me/alpha/k8s", 202: "/home/me/beta"}[pid]
	}
	env := func(pid int) int {
		if pid == 202 {
			return 10360
		}
		return 0
	}

	got := parseTiltProcesses(ps, cwd, env)
	if len(got) != 2 {
		t.Fatalf("got %d instances, want 2: %+v", len(got), got)
	}
	// pid 101: port from args, label from cwd (k8s is generic -> alpha).
	if got[0] != (Instance{Host: "localhost", Port: 10351, Label: "alpha", PID: 101}) {
		t.Errorf("instance 0 = %+v", got[0])
	}
	// pid 202: no port flag, so env(202) supplies 10360; label beta.
	if got[1] != (Instance{Host: "localhost", Port: 10360, Label: "beta", PID: 202}) {
		t.Errorf("instance 1 = %+v", got[1])
	}
}
