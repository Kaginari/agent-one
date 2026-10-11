package sandbox

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// onOS makes the sandbox believe it runs on goos for the rest of the test.
func onOS(t *testing.T, goos string) {
	t.Helper()
	old := hostOS
	hostOS = goos
	t.Cleanup(func() { hostOS = old })
}

// fakeProbes replaces both probes; each records that it ran.
func fakeProbes(t *testing.T, bw, sb Probe) (bwRan, sbRan *bool) {
	t.Helper()
	ob, os_ := probeBwrap, probeSeatbelt
	bwRan, sbRan = new(bool), new(bool)
	probeBwrap = func(string, time.Duration) Probe { *bwRan = true; return bw }
	probeSeatbelt = func(string, time.Duration) Probe { *sbRan = true; return sb }
	t.Cleanup(func() { probeBwrap, probeSeatbelt = ob, os_ })
	return bwRan, sbRan
}

// The fence follows the OS: Seatbelt on darwin, bwrap on linux, none elsewhere — and a refusing
// sandbox-exec leaves none with its reason for `status`.
func TestModeFollowsTheOS(t *testing.T) {
	okSB := Probe{Mode: Seatbelt, Available: true, Path: "/usr/bin/sandbox-exec", Version: "sandbox-exec"}
	okBW := Probe{Mode: Bwrap, Available: true, Path: "/usr/bin/bwrap", Version: "bubblewrap 0.9"}

	onOS(t, "darwin")
	bw, sb := fakeProbes(t, okBW, okSB)
	s := New(Options{Enabled: true, Root: t.TempDir()})
	if s.Mode() != Seatbelt || *bw || !*sb {
		t.Fatalf("darwin: mode %s (bwrap probed %v, seatbelt probed %v)", s.Mode(), *bw, *sb)
	}
	if got := s.Status(); got != "sandbox: seatbelt (sandbox-exec)" {
		t.Fatalf("status: %q", got)
	}
	w := s.Wrap(Call{}, []string{"echo", "hi"})
	if w[0] != "/usr/bin/sandbox-exec" || w[1] != "-p" || !strings.HasPrefix(w[2], "(version 1)") {
		t.Fatalf("wrap: %q", w[:3])
	}
	if n := len(w); w[n-3] != "--" || w[n-2] != "echo" || w[n-1] != "hi" {
		t.Fatalf("argv must follow --: %q", w[n-3:])
	}
	for _, a := range w[3 : len(w)-3] {
		if !strings.HasPrefix(a, "-D") || !strings.Contains(a, "=") {
			t.Fatalf("between the profile and -- only -DKEY=VALUE: %q", a)
		}
	}

	fakeProbes(t, okBW, Probe{Mode: Seatbelt, Path: "/usr/bin/sandbox-exec", Why: "sandbox-exec refused: syntax error"})
	s = New(Options{Enabled: true})
	if s.Mode() != None || s.Why() != "sandbox-exec refused: syntax error" || !strings.Contains(s.Status(), "refused") {
		t.Fatalf("refused: %s %q", s.Mode(), s.Why())
	}
	if got := s.Wrap(Call{}, []string{"ls"}); len(got) != 1 {
		t.Fatalf("none wraps nothing: %q", got)
	}
	if New(Options{Enabled: false}).Mode() != None {
		t.Fatal("tools.bash.sandbox: none turns Seatbelt off")
	}

	onOS(t, "linux")
	bw, sb = fakeProbes(t, okBW, okSB)
	if s := New(Options{Enabled: true}); s.Mode() != Bwrap || !*bw || *sb {
		t.Fatalf("linux: mode %s (bwrap probed %v, seatbelt probed %v)", s.Mode(), *bw, *sb)
	}

	onOS(t, "freebsd")
	bw, sb = fakeProbes(t, okBW, okSB)
	if s := New(Options{Enabled: true}); s.Mode() != None || *bw || *sb || !strings.Contains(s.Why(), "freebsd") {
		t.Fatalf("freebsd: %s %q", s.Mode(), s.Why())
	}
}

func TestProbeSeatbeltMissingBinary(t *testing.T) {
	p := ProbeSeatbelt("/nonexistent/sandbox-exec", time.Second)
	if p.Available || p.Mode != Seatbelt || !strings.Contains(p.Why, "not found") {
		t.Fatalf("%+v", p)
	}
}

// block returns the lines of the profile from the one starting with head to the paren closing it.
func block(t *testing.T, prof, head string) string {
	t.Helper()
	i := strings.Index(prof, head)
	if i < 0 {
		t.Fatalf("no %q in profile:\n%s", head, prof)
	}
	depth := 0
	for j, r := range prof[i:] {
		switch r {
		case '(':
			depth++
		case ')':
			depth--
			if depth == 0 {
				return prof[i : i+j+1]
			}
		}
	}
	t.Fatalf("unbalanced %q", head)
	return ""
}

func paramMap(params []string) map[string]string {
	m := map[string]string{}
	for _, kv := range params {
		k, v, _ := strings.Cut(kv, "=")
		m[k] = v
	}
	return m
}

// Writes are allowed exactly under the root, each read-write path, the temp dirs and the devices.
func TestSeatbeltProfileWrites(t *testing.T) {
	prof, params := seatbeltProfile(fence{Root: "/w/workspace", RW: []string{"/m/shared", "/d/data"}, Secrets: []string{"/h/.ssh"}})
	allow := block(t, prof, "(allow file-write*")
	want := []string{
		`(subpath (param "ROOT"))`, `(subpath (param "RW_0"))`, `(subpath (param "RW_1"))`,
		`(subpath "/private/tmp")`, `(subpath "/private/var/folders")`,
		`(literal "/dev/null")`, `(literal "/dev/zero")`, `(literal "/dev/stdout")`, `(literal "/dev/stderr")`,
		`(literal "/dev/dtracehelper")`, `(literal "/dev/ptmx")`, `(regex #"^/dev/fd/")`, `(regex #"^/dev/tty")`,
	}
	got := strings.Fields(strings.NewReplacer("(allow file-write*", "", "\n", " ").Replace(allow))
	if strings.Join(got, " ") != strings.Join(strings.Fields(strings.Join(want, " ")), " ")+")" {
		t.Fatalf("write allowances:\n%s", allow)
	}
	if strings.Count(prof, "(allow file-write*") != 1 || !strings.Contains(prof, "(deny file-write*)\n(allow file-write*") {
		t.Fatalf("one deny-then-allow for writes:\n%s", prof)
	}
	m := paramMap(params)
	if m["ROOT"] != "/w/workspace" || m["RW_0"] != "/m/shared" || m["RW_1"] != "/d/data" || m["SECRET_0"] != "/h/.ssh" || len(m) != 4 {
		t.Fatalf("params: %q", params)
	}
	for _, r := range prof {
		if r > 0x7e || r < 0x20 && r != '\n' {
			t.Fatalf("the profile is plain ASCII (sandbox-exec's parser): %q", r)
		}
	}
	if strings.Count(prof, "(") != strings.Count(prof, ")") {
		t.Fatalf("unbalanced profile:\n%s", prof)
	}

	// no root, no extra paths: only the temp dirs and devices
	bare, bp := seatbeltProfile(fence{})
	if strings.Contains(bare, "param") || len(bp) != 0 {
		t.Fatalf("an empty fence names no parameter:\n%s %q", bare, bp)
	}
}

// Without the gate's outward verdict IP is denied (loopback and unix sockets aside) and DNS is
// closed; with it the network stanza is gone.
func TestSeatbeltProfileNetwork(t *testing.T) {
	off, _ := seatbeltProfile(fence{Root: "/w"})
	for _, w := range []string{"(deny network*)", `(local ip "localhost:*")`, `(remote ip "localhost:*")`, "(remote unix-socket)",
		`(deny network-outbound (remote unix-socket (path-literal "/private/var/run/mDNSResponder")))`} {
		if !strings.Contains(off, w) {
			t.Fatalf("no-network profile lacks %s:\n%s", w, off)
		}
	}
	allow := block(t, off, "(allow network*")
	if strings.Count(allow, " ip ") != 2 || strings.Count(allow, "localhost:*") != 2 || strings.Contains(allow, `"*:`) {
		t.Fatalf("only loopback IP may be allowed:\n%s", allow)
	}
	if strings.Index(off, "(deny network*)") > strings.Index(off, "(allow network*") {
		t.Fatal("the deny must come before its allowances (the last matching rule wins)")
	}
	on, _ := seatbeltProfile(fence{Root: "/w", Network: true})
	if strings.Contains(on, "network") {
		t.Fatalf("an outward call keeps the network:\n%s", on)
	}
}

// Every credential path — the shared list and macOS's own, as written and as resolved — is
// denied to reads and writes, in the profile's last rule so nothing re-allows it.
func TestSeatbeltProfileDeniesEverySecret(t *testing.T) {
	home := t.TempDir()
	keys := t.TempDir()
	if err := os.Symlink(keys, filepath.Join(home, ".ssh")); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(home, ".aws"), 0o700); err != nil {
		t.Fatal(err)
	}
	f := fenceFor(Options{Root: t.TempDir()}, Call{}, home)
	prof, params := seatbeltProfile(f)
	m := paramMap(params)
	denied := map[string]bool{}
	for k, v := range m {
		if strings.HasPrefix(k, "SECRET_") {
			denied[v] = true
		}
	}
	rhome := realPath(home)
	for _, rel := range append(append([]string(nil), secretHomes...), macSecretHomes...) {
		if !denied[filepath.Join(rhome, rel)] {
			t.Errorf("%s not denied", rel)
		}
	}
	if !denied[realPath(keys)] {
		t.Errorf("the target of the ~/.ssh symlink (%s) is not denied", keys)
	}
	deny := block(t, prof, "(deny file-read* file-write*")
	if !strings.HasSuffix(strings.TrimSpace(prof), deny) {
		t.Fatalf("the credential deny must be the last rule:\n%s", prof)
	}
	for k := range m {
		if strings.HasPrefix(k, "SECRET_") && !strings.Contains(deny, `(subpath (param "`+k+`"))`) {
			t.Errorf("%s defined but not denied", k)
		}
	}
	for _, rel := range []string{"Library/Keychains", "Library/Application Support/Google/Chrome", "Library/Application Support/Firefox", "Library/Safari"} {
		if !denied[filepath.Join(rhome, rel)] {
			t.Errorf("macOS secret %s not denied", rel)
		}
	}
}

// A path is a -D parameter, never profile text: a root carrying SBPL cannot change the profile.
func TestSeatbeltPathsCannotInjectSBPL(t *testing.T) {
	evil := "/w/a\"b)\n(allow file-write* (subpath \"/\"))\n; x"
	plain, pp := seatbeltProfile(fence{Root: "/w/plain", RW: []string{"/r"}, Secrets: []string{"/h/.ssh"}})
	bad, bp := seatbeltProfile(fence{Root: evil, RW: []string{evil + "2"}, Secrets: []string{evil + "3"}})
	if plain != bad {
		t.Fatalf("the profile text changed with the path's characters:\n%s", bad)
	}
	if paramMap(bp)["ROOT"] != evil || paramMap(bp)["RW_0"] != evil+"2" || paramMap(bp)["SECRET_0"] != evil+"3" || len(pp) != len(bp) {
		t.Fatalf("the path must pass through untouched: %q", bp)
	}
	if strings.Contains(bad, "/w/a") {
		t.Fatal("a path leaked into the profile text")
	}
	// and the whole argv keeps each -D one argument
	args := []string{"-p", bad}
	for _, kv := range bp {
		args = append(args, "-D"+kv)
	}
	if args[2] != "-DROOT="+evil {
		t.Fatalf("%q", args[2])
	}
}

// Seatbelt matches real paths: a symlinked root, or a not-yet-made path under one, resolves.
func TestRealPathResolves(t *testing.T) {
	base := realPath(t.TempDir())
	real := filepath.Join(base, "real")
	if err := os.Mkdir(real, 0o755); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(base, "link")
	if err := os.Symlink(real, link); err != nil {
		t.Fatal(err)
	}
	if got := realPath(link); got != real {
		t.Fatalf("%s → %s", link, got)
	}
	if got := realPath(filepath.Join(link, "new", "deeper")); got != filepath.Join(real, "new", "deeper") {
		t.Fatalf("missing tail: %s", got)
	}
	f := fenceFor(Options{Root: link, RWPaths: []string{link, filepath.Join(base, "absent"), ""}}, Call{}, "")
	if f.Root != real || len(f.RW) != 1 || f.RW[0] != real {
		t.Fatalf("fence: %+v (absent read-write paths are skipped, as under bwrap)", f)
	}
}

// The profile for a sample workspace, for reading: go test ./sandbox/ -run SeatbeltProfileSample -v
func TestSeatbeltProfileSample(t *testing.T) {
	prof, params := seatbeltProfile(fence{Root: "/Users/me/src/workspace", RW: []string{"/Users/me/.local/share/agent-one"},
		Secrets: []string{"/Users/me/.ssh", "/Users/me/Library/Keychains"}})
	t.Logf("sandbox-exec -p '<profile>' %s -- argv…\n%s", "-D"+strings.Join(params, " -D"), prof)
}
