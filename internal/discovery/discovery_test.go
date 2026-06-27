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

	noTiltfile := func(string) bool { return false }
	got := parseTiltProcesses(ps, cwd, env, noTiltfile)
	if len(got) != 2 {
		t.Fatalf("got %d instances, want 2: %+v", len(got), got)
	}
	// pid 101: port from args, label from cwd (k8s is generic -> alpha).
	if got[0] != (Instance{Host: "localhost", Port: 10351, Label: "alpha", Dir: "/home/me/alpha/k8s", PID: 101}) {
		t.Errorf("instance 0 = %+v", got[0])
	}
	// pid 202: no port flag, so env(202) supplies 10360; label beta.
	if got[1] != (Instance{Host: "localhost", Port: 10360, Label: "beta", Dir: "/home/me/beta", PID: 202}) {
		t.Errorf("instance 1 = %+v", got[1])
	}
}

func TestIsTiltLauncher(t *testing.T) {
	yes := [][]string{
		{"task", "up"},  // a task runner in a stack dir — any target counts
		{"task", "dev"}, // an earlier setup phase is still the stack starting
		{"make", "tilt-up"},
		{"just", "up"},
		{"./tilt.sh"}, // not a task runner, but references tilt
		{"bash", "run-tilt.sh"},
	}
	no := [][]string{
		{"tilt", "up", "--port", "10350"}, // the tilt binary itself (isTiltUp's job)
		{"lazytilt"},                      // us
		{"vim", "Tiltfile"},               // editing the file isn't launching
		{"go", "build", "./..."},          // not a task runner, no tilt reference
		{},                                // empty
	}
	for _, a := range yes {
		if !isTiltLauncher(a) {
			t.Errorf("isTiltLauncher(%v) = false, want true", a)
		}
	}
	for _, a := range no {
		if isTiltLauncher(a) {
			t.Errorf("isTiltLauncher(%v) = true, want false", a)
		}
	}
}

// TestParseTiltProcessesStarting covers launcher detection: a task runner counts
// as a still-starting stack only when its dir holds a Tiltfile.
func TestParseTiltProcessesStarting(t *testing.T) {
	ps := "  501 task up\n" + // launcher, dir has Tiltfile -> starting
		"  502 task up\n" + // launcher, dir has no Tiltfile -> ignored
		"  503 go build ./...\n" // not a launcher, even in a stack dir -> ignored
	cwd := func(pid int) string {
		return map[int]string{501: "/home/me/app-two", 502: "/home/me/elsewhere", 503: "/home/me/app-two"}[pid]
	}
	hasTiltfile := func(dir string) bool { return dir == "/home/me/app-two" }

	got := parseTiltProcesses(ps, cwd, nil, hasTiltfile)
	if len(got) != 1 {
		t.Fatalf("got %d instances, want 1: %+v", len(got), got)
	}
	want := Instance{Host: "localhost", Label: "app-two", Dir: "/home/me/app-two", PID: 501, Starting: true}
	if got[0] != want {
		t.Errorf("starting instance = %+v, want %+v", got[0], want)
	}
}

// TestMergeInstances covers the dedup: running instances sort by port first, a
// launcher whose dir already has a live tilt is dropped, and duplicate launchers
// for one dir collapse to a single still-starting row.
func TestMergeInstances(t *testing.T) {
	scanned := []Instance{
		{Host: "localhost", Port: 10360, Label: "app-two", Dir: "/app-2", PID: 2},
		{Host: "localhost", Port: 10350, Label: "app-one", Dir: "/app-1", PID: 1},
		{Host: "localhost", Label: "app-one", Dir: "/app-1", PID: 3, Starting: true},   // dropped: /app-1 is live
		{Host: "localhost", Label: "app-three", Dir: "/app-3", PID: 4, Starting: true}, // kept
		{Host: "localhost", Label: "app-three", Dir: "/app-3", PID: 5, Starting: true}, // dup dir -> collapsed
		{Host: "localhost", Label: "app-four", Dir: "/app-4", PID: 6, Starting: true},  // kept
	}
	got := mergeInstances(scanned)
	if len(got) != 4 {
		t.Fatalf("got %d instances, want 4: %+v", len(got), got)
	}
	// Running first, by port.
	if got[0].Port != 10350 || got[1].Port != 10360 {
		t.Errorf("running not sorted by port: %+v", got[:2])
	}
	// Then starting, by dir: /app-3 before /app-4.
	if !got[2].Starting || got[2].Dir != "/app-3" {
		t.Errorf("got[2] = %+v, want starting /app-3", got[2])
	}
	if !got[3].Starting || got[3].Dir != "/app-4" {
		t.Errorf("got[3] = %+v, want starting /app-4", got[3])
	}
}
