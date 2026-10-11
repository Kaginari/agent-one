//go:build darwin

package sandbox

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

// On a Mac, Seatbelt must be chosen and must hold: the workspace writable, the home and the system
// not, credentials unreadable, no network unless outward, the host's processes not signalable.
// Run on a Mac: go test ./sandbox/ -run Seatbelt -v
func TestSeatbeltConfinement(t *testing.T) {
	realHome, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}
	home := t.TempDir() // under /private/var/folders: writable, so the secret deny must win over it
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
	if sb.Mode() != Seatbelt {
		t.Fatalf("Seatbelt not chosen on this Mac: %s — %s", sb.Mode(), sb.Why())
	}
	if sb.Status() != "sandbox: seatbelt (sandbox-exec)" {
		t.Fatal(sb.Status())
	}
	run := func(call Call, script string) (string, error) {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		call.Cwd = root
		env := []string{"PATH=/usr/bin:/bin:/usr/sbin:/sbin", "HOME=" + home, "MY_TOKEN=leak", "SAFE=1"}
		out, err := sb.Command(ctx, call, env, "/bin/bash", "-c", script).CombinedOutput()
		return strings.TrimSpace(string(out)), err
	}

	if out, err := run(Call{}, "echo hi > inside.txt && cat inside.txt"); err != nil || out != "hi" {
		t.Fatalf("the root must be writable: %q %v", out, err)
	}
	if _, err := os.Stat(filepath.Join(root, "inside.txt")); err != nil {
		t.Fatal("the write did not land on the host")
	}
	if out, err := run(Call{}, "echo x > /tmp/agent-one-sb-$$ && cat /tmp/agent-one-sb-$$ && rm /tmp/agent-one-sb-$$"); err != nil || out != "x" {
		t.Fatalf("/tmp must be writable: %q %v", out, err)
	}
	if out, err := run(Call{}, `echo x > "$TMPDIR/agent-one-sb" && rm "$TMPDIR/agent-one-sb"`); err != nil {
		t.Fatalf("$TMPDIR must be writable: %q %v", out, err)
	}
	probe := filepath.Join(realHome, "agent-one-seatbelt-should-fail-"+strconv.Itoa(os.Getpid()))
	defer os.Remove(probe)
	if out, err := run(Call{}, "touch "+shq(probe)); err == nil {
		t.Fatalf("a write to the real home must fail: %q", out)
	}
	if _, err := os.Stat(probe); err == nil {
		t.Fatal("the write to the home landed")
	}
	if _, err := run(Call{}, "touch /usr/local/agent-one-should-fail || touch /Library/agent-one-should-fail"); err == nil {
		t.Fatal("the system must be read-only")
	}
	if out, _ := run(Call{}, "cat ~/.ssh/id_ed25519 ~/.netrc 2>&1"); strings.Contains(out, "SECRET") {
		t.Fatalf("a credential was readable: %q", out)
	}
	if out, err := run(Call{}, "ls -A ~/.ssh"); err == nil && strings.Contains(out, "id_ed25519") {
		t.Fatalf("a credential directory was listable: %q", out)
	}
	if out, err := run(Call{}, "echo k > ~/.ssh/authorized_keys"); err == nil {
		t.Fatalf("a credential path was writable: %q", out)
	}
	if out, _ := run(Call{}, `echo "${MY_TOKEN:-scrubbed}" "$SAFE"`); out != "scrubbed 1" {
		t.Fatalf("env scrub: %q", out)
	}
	if out, err := run(Call{}, "kill -0 "+strconv.Itoa(os.Getpid())); err == nil {
		t.Fatalf("a process outside the sandbox must not be signalable: %q", out)
	}
	if out, err := run(Call{}, "sleep 0 & wait $!; kill -0 $$"); err != nil {
		t.Fatalf("the sandbox's own processes stay signalable: %q %v", out, err)
	}

	const fetch = "curl -fsS -m 10 -o /dev/null https://example.com"
	if _, err := exec.LookPath("curl"); err != nil {
		t.Skip("no curl")
	}
	if out, err := run(Call{}, fetch); err == nil {
		t.Fatalf("the network must be off without the outward verdict: %q", out)
	}
	if out, err := run(Call{Network: true}, fetch); err != nil {
		if c := exec.Command("/bin/sh", "-c", fetch); c.Run() != nil {
			t.Skip("no internet from this Mac at all: " + out)
		}
		t.Fatalf("an outward call must reach the network: %q %v", out, err)
	}
}

func shq(s string) string { return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'" }
