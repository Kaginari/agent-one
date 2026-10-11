package sandbox

// Seatbelt is macOS's native fence: /usr/bin/sandbox-exec applies an SBPL profile to itself
// and execs the command, which inherits it with every child.
//
// Why the profile starts from (allow default) and fences what matters — rather than the
// (deny default) of Codex CLI and Claude Code's sandbox: a deny-default profile must enumerate
// every mach service, sysctl, IOKit class and file a toolchain touches, and agent-one runs
// whatever the workspace builds with — Xcode and its simulators, JVMs, Homebrew toolchains, Node's
// native modules — which no list here can foresee; a missing allowance fails as an obscure
// "Operation not permitted" deep inside a build. Bazel's darwin sandbox makes the same choice
// ((allow default), then (deny file-write*) and (deny network*) with allowances) and runs
// arbitrary toolchains under it. The fence mirrors bwrap's: writes only under the workspace, its
// extra read-write paths and the temp dirs; no network but loopback and unix sockets; the home's
// credentials unreadable; processes outside the sandbox neither inspectable nor signalable;
// plus the two well-known ways out of an allow-default profile closed (Apple Events to another
// app, LaunchServices' `open`). What it does not close is documented in the README: mach IPC
// stays open, so it is a fence against a model's mistakes, not a jail for a hostile binary.
//
// Paths never enter the profile text: each is a -D KEY=VALUE parameter read with (param "KEY"),
// so a workspace root holding `"`, `)` or a newline is data, never SBPL.

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// SeatbeltBin is the only sandbox-exec trusted: never one found on PATH.
var SeatbeltBin = "/usr/bin/sandbox-exec"

// macSecretHomes are the macOS-only credential paths under the home, denied beside secretHomes:
// the keychains, cookies and the browsers' profiles (saved passwords, session cookies).
var macSecretHomes = []string{
	"Library/Keychains", "Library/Cookies", "Library/Safari", "Library/Containers/com.apple.Safari",
	"Library/Application Support/Google/Chrome", "Library/Application Support/Chromium",
	"Library/Application Support/BraveSoftware", "Library/Application Support/Microsoft Edge",
	"Library/Application Support/Firefox", "Library/Application Support/Arc",
}

// ProbeSeatbelt runs /usr/bin/true under sandbox-exec with the very profile this package
// writes (for a throwaway root), so a profile this macOS refuses shows as none, with why.
func ProbeSeatbelt(bin string, timeout time.Duration) Probe {
	if bin == "" {
		bin = SeatbeltBin
	}
	if _, err := os.Stat(bin); err != nil {
		return Probe{Mode: Seatbelt, Why: "sandbox-exec not found at " + bin}
	}
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	args := append(seatbeltArgs(Options{Root: os.TempDir()}, Call{}), "--", "/usr/bin/true")
	out, err := exec.CommandContext(ctx, bin, args...).CombinedOutput()
	if err != nil {
		why := strings.TrimSpace(string(out))
		if why == "" {
			why = err.Error()
		}
		return Probe{Mode: Seatbelt, Path: bin, Version: "sandbox-exec", Why: "sandbox-exec refused: " + firstLine(why)}
	}
	return Probe{Mode: Seatbelt, Available: true, Path: bin, Version: "sandbox-exec"}
}

// fence is what a profile is parameterised by: real paths (symlinks resolved, as Seatbelt
// matches them — /tmp is /private/tmp, /var is /private/var on a Mac).
type fence struct {
	Root    string   // "" = no workspace root
	RW      []string // the extra read-write paths that exist
	Secrets []string // the credential paths to deny, existing or not
	Network bool
}

// seatbeltArgs is sandbox-exec's arguments before "--": the profile, then its parameters.
func seatbeltArgs(opt Options, call Call) []string {
	home, _ := os.UserHomeDir()
	prof, params := seatbeltProfile(fenceFor(opt, call, home))
	args := []string{"-p", prof}
	for _, kv := range params {
		args = append(args, "-D"+kv)
	}
	return args
}

func fenceFor(opt Options, call Call, home string) fence {
	f := fence{Root: realPath(opt.Root), Network: call.Network}
	for _, p := range opt.RWPaths {
		if p == "" {
			continue
		}
		if _, err := os.Stat(p); err == nil {
			f.RW = append(f.RW, realPath(p))
		}
	}
	f.Secrets = secretPaths(home)
	return f
}

// secretPaths is every credential path under home, each as written and, when it is a symlink,
// where it leads — Seatbelt sees the resolved path. Paths not there yet are denied too: a
// command must not create ~/.aws/credentials for a later tool to trust either.
func secretPaths(home string) []string {
	if home == "" || home == "/" {
		return nil
	}
	home = realPath(home)
	var out []string
	seen := map[string]bool{}
	add := func(p string) {
		if p != "" && !seen[p] {
			seen[p] = true
			out = append(out, p)
		}
	}
	for _, rel := range append(append([]string(nil), secretHomes...), macSecretHomes...) {
		p := filepath.Join(home, rel)
		add(p)
		add(realPath(p))
	}
	return out
}

// realPath is p absolute with its symlinks resolved; a path not there yet keeps its missing
// tail on its deepest existing ancestor's real path (/tmp/new → /private/tmp/new on a Mac).
func realPath(p string) string {
	if p == "" {
		return ""
	}
	if a, err := filepath.Abs(p); err == nil {
		p = a
	}
	if r, err := filepath.EvalSymlinks(p); err == nil {
		return r
	}
	dir, base := filepath.Split(p)
	dir = filepath.Clean(dir)
	if dir == p || base == "" {
		return p
	}
	return filepath.Join(realPath(dir), base)
}

// seatbeltProfile writes the SBPL profile for a fence and the KEY=VALUE parameters it reads.
// The text depends only on the shape of the fence (is there a root, how many paths, network or
// not), never on a path's characters.
func seatbeltProfile(f fence) (string, []string) {
	var params []string
	param := func(key, val string) string {
		params = append(params, key+"="+val)
		return fmt.Sprintf(`(param %q)`, key)
	}
	var b strings.Builder
	w := func(s ...string) {
		for _, l := range s {
			b.WriteString(l)
			b.WriteByte('\n')
		}
	}
	w("(version 1)",
		"; allow by default: a deny-default profile breaks toolchains it cannot foresee",
		"(allow default)",
		"",
		"; writes: the workspace, its read-write paths, the temp dirs, the terminal and null devices",
		"(deny file-write*)",
		"(allow file-write*")
	if f.Root != "" {
		w("  (subpath " + param("ROOT", f.Root) + ")")
	}
	for i, p := range f.RW {
		w("  (subpath " + param(fmt.Sprintf("RW_%d", i), p) + ")")
	}
	w(`  (subpath "/private/tmp")`,
		`  (subpath "/private/var/folders")`,
		`  (literal "/dev/null")`,
		`  (literal "/dev/zero")`,
		`  (literal "/dev/stdout")`,
		`  (literal "/dev/stderr")`,
		`  (literal "/dev/dtracehelper")`,
		`  (literal "/dev/ptmx")`,
		`  (regex #"^/dev/fd/")`,
		`  (regex #"^/dev/tty"))`)
	if !f.Network {
		w("",
			"; network: none but loopback and local unix sockets (a test's own server, a build daemon);",
			"; DNS through mDNSResponder is closed too, so names cannot carry data out",
			"(deny network*)",
			"(allow network*",
			`  (local ip "localhost:*")`,
			`  (remote ip "localhost:*")`,
			`  (remote unix-socket))`,
			`(allow network-bind (local unix-socket (subpath "/")))`,
			`(deny network-outbound (remote unix-socket (path-literal "/private/var/run/mDNSResponder")))`)
	}
	w("",
		"; processes: no pid namespace on macOS - others stay listed (ps) but cannot be",
		"; inspected or signalled from inside",
		"(deny process-info*)",
		"(allow process-info* (target same-sandbox))",
		"(deny signal)",
		"(allow signal (target same-sandbox))",
		"",
		"; the ways out of an allow-default profile: driving another app, LaunchServices (open),",
		"; and the clipboard",
		"(deny appleevent-send)",
		"(deny mach-lookup",
		`  (global-name "com.apple.coreservices.launchservicesd")`,
		`  (global-name "com.apple.pasteboard.1"))`)
	if len(f.Secrets) > 0 {
		w("",
			"; credentials last, so no allowance above reopens them",
			"(deny file-read* file-write*")
		for i, p := range f.Secrets {
			end := ")"
			if i == len(f.Secrets)-1 {
				end = "))"
			}
			w("  (subpath " + param(fmt.Sprintf("SECRET_%d", i), p) + end)
		}
	}
	return b.String(), params
}
