package sandbox

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestScrubEnv(t *testing.T) {
	env := []string{"HOME=/h", "ANTHROPIC_API_KEY=sk", "GITHUB_TOKEN=x", "DB_SECRET=y", "PG_PASSWORD=z", "api_key=lower", "TOKEN=bare", "AWS_ACCESS_KEY_ID=a", "KEEP_TOKEN=k", "PATH=/bin", "NOTASECRET=1"}
	got := ScrubEnv(env, []string{"AWS_*"}, []string{"KEEP_TOKEN"})
	want := "HOME=/h PATH=/bin NOTASECRET=1 KEEP_TOKEN=k"
	for _, w := range strings.Fields(want) {
		if !contains(got, w) {
			t.Errorf("missing %s in %v", w, got)
		}
	}
	if len(got) != 4 {
		t.Errorf("want 4 survivors, got %v", got)
	}
}

func contains(xs []string, x string) bool {
	for _, y := range xs {
		if y == x {
			return true
		}
	}
	return false
}

func TestNoneMode(t *testing.T) {
	s := New(Options{Enabled: false})
	if s.Mode() != None || s.Why() != "disabled by config" {
		t.Fatalf("%s %s", s.Mode(), s.Why())
	}
	if got := s.Wrap(Call{}, []string{"ls"}); len(got) != 1 {
		t.Fatal("none mode wraps nothing")
	}
	var nilS *Sandbox
	if nilS.Mode() != None || nilS.Status() == "" {
		t.Fatal("nil sandbox is None")
	}
	onOS(t, "linux")
	bad := New(Options{Enabled: true, Bwrap: "/nonexistent/bwrap"})
	if bad.Mode() != None || bad.Why() == "" {
		t.Fatalf("missing binary: %s %q", bad.Mode(), bad.Why())
	}
}

func requireBwrap(t *testing.T, root string) *Sandbox {
	t.Helper()
	s := New(Options{Enabled: true, Root: root})
	if s.Mode() != Bwrap {
		t.Skip("bwrap unavailable: " + s.Why())
	}
	return s
}

func TestBwrapConfinement(t *testing.T) {
	root := t.TempDir()
	s := requireBwrap(t, root)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	run := func(call Call, script string) (string, error) {
		cmd := s.Command(ctx, call, []string{"PATH=/usr/bin:/bin", "MY_TOKEN=leak", "SAFE=1"}, "bash", "-c", script)
		out, err := cmd.CombinedOutput()
		return strings.TrimSpace(string(out)), err
	}
	if out, err := run(Call{Cwd: root}, "echo hi > inside.txt && cat inside.txt"); err != nil || out != "hi" {
		t.Fatalf("root is rw: %q %v", out, err)
	}
	if _, err := os.Stat(filepath.Join(root, "inside.txt")); err != nil {
		t.Fatal("write did not land on the host")
	}
	if _, err := run(Call{Cwd: root}, "touch /usr/agent-one-should-fail"); err == nil {
		t.Fatal("/ must be read-only")
	}
	if out, err := run(Call{Cwd: root}, "echo x > /tmp/priv && cat /tmp/priv"); err != nil || out != "x" {
		t.Fatalf("private /tmp: %q %v", out, err)
	}
	if _, err := os.Stat("/tmp/priv"); err == nil {
		t.Fatal("/tmp must be private")
	}
	if out, _ := run(Call{Cwd: root}, "echo \"${MY_TOKEN:-scrubbed}\" \"$SAFE\""); out != "scrubbed 1" {
		t.Fatalf("env scrub: %q", out)
	}
	ifaces := "tail -n +3 /proc/net/dev | cut -d: -f1 | tr -d ' ' | grep -v '^lo$' | wc -l"
	if out, _ := run(Call{Cwd: root}, ifaces); out != "0" {
		t.Fatalf("network should be unshared, saw interfaces: %q", out)
	}
	if out, _ := run(Call{Cwd: root, Network: true}, ifaces); out == "0" {
		t.Log("network call sees no interface beyond lo (host may have none)")
	}
	if !strings.Contains(s.Status(), "bwrap") {
		t.Fatal(s.Status())
	}
}

func TestEnvSetWinsAndTokenAllowed(t *testing.T) {
	s := New(Options{EnvAllow: []string{"ARTIFACTORY_TOKEN"}, EnvSet: []string{"NPM_CONFIG_REGISTRY=https://art/npm/"}})
	got := strings.Join(s.Scrub([]string{"NPM_CONFIG_REGISTRY=https://registry.npmjs.org/", "ARTIFACTORY_TOKEN=t", "OTHER_TOKEN=x", "PATH=/bin"}), " ")
	if !strings.Contains(got, "NPM_CONFIG_REGISTRY=https://art/npm/") || strings.Contains(got, "registry.npmjs.org") {
		t.Fatalf("the registry's value wins: %s", got)
	}
	if !strings.Contains(got, "ARTIFACTORY_TOKEN=t") || strings.Contains(got, "OTHER_TOKEN") {
		t.Fatalf("the named token passes, other secrets do not: %s", got)
	}
}

// A command that backgrounds a child holding its output must not keep Run past the deadline.
func TestCommandDeadlineKillsTheTree(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	var nilbox *Sandbox
	cmd := nilbox.Command(ctx, Call{Cwd: t.TempDir()}, nil, "bash", "-c", "sleep 30 & sleep 30")
	var out bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &out
	start := time.Now()
	_ = cmd.Run()
	if d := time.Since(start); d > WaitDelay+2*time.Second {
		t.Fatalf("Run returned after %s — the backgrounded child kept it", d)
	}
}

// Credentials named the common ways, or carried in a URL's userinfo, never reach a child.
func TestScrubCommonCredentialNames(t *testing.T) {
	env := []string{"AWS_SECRET_ACCESS_KEY=a", "GH_PRIVATE_KEY=b", "GOOGLE_APPLICATION_CREDENTIALS=c", "DOCKER_CREDENTIAL=d",
		"SESSION_TOKEN_V2=e", "DATABASE_URL=postgres://u:p@db/x", "REDIS_URL=redis://:pw@cache:6379", "PUBLIC_URL=https://example.com/a@b", "NOTASECRET=1", "TOKENIZER=bpe"}
	got := strings.Join(ScrubEnv(env, nil, nil), " ")
	if got != "PUBLIC_URL=https://example.com/a@b NOTASECRET=1 TOKENIZER=bpe" {
		t.Fatalf("scrubbed to %q", got)
	}
}

// Inside the sandbox the home's credentials read as nothing and the host's processes are out of
// sight, while the workspace itself stays writable.
func TestBwrapMasksSecretsAndHostProcesses(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	if err := os.MkdirAll(filepath.Join(home, ".ssh"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, ".ssh", "id_ed25519"), []byte("SECRET-KEY"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, ".netrc"), []byte("machine x password SECRET"), 0o600); err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	sb := New(Options{Enabled: true, Root: root, Timeout: 10 * time.Second})
	if sb.Mode() != Bwrap {
		t.Skip("bwrap unavailable: " + sb.Why())
	}
	run := func(script string) string {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		out, _ := sb.Command(ctx, Call{Cwd: root}, os.Environ(), "bash", "-c", script).CombinedOutput()
		return string(out)
	}
	if out := run("cat ~/.ssh/id_ed25519 ~/.netrc 2>/dev/null; ls -A ~/.ssh"); strings.Contains(out, "SECRET") || strings.Contains(out, "id_ed25519") {
		t.Fatalf("a credential was readable inside the sandbox: %q", out)
	}
	// in its own pid namespace the command sees bwrap's init, the shell and ls — a handful, never the host
	if n, err := strconv.Atoi(strings.TrimSpace(run("ls /proc | grep -c '^[0-9]'"))); err != nil || n > 8 {
		t.Fatalf("the host's processes are visible inside the sandbox: %d pids (%v)", n, err)
	}
	if out := run("echo ok > inside.txt && cat inside.txt"); !strings.Contains(out, "ok") {
		t.Fatalf("the workspace must stay writable: %q", out)
	}
}
