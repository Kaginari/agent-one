// Package sandbox runs a command fenced: under bubblewrap on Linux, under Seatbelt
// (sandbox-exec) on macOS. Either way the filesystem is read-only except the workspace root and the
// temp dirs, the home's credential paths read as nothing, and there is no network unless the call
// passed the human gate as outward. Where the fence is missing or cannot run (some kernels and
// containers refuse user namespaces; a Mac whose sandbox-exec refuses the profile) the mode is
// None and the reason is kept for `status`. Secrets never cross: the environment is scrubbed
// before any child sees it.
package sandbox

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path"
	"regexp"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"time"
)

// Mode is how a command is contained.
type Mode string

const (
	Bwrap    Mode = "bwrap"    // Linux: bubblewrap
	Seatbelt Mode = "seatbelt" // macOS: sandbox-exec with an SBPL profile
	None     Mode = "none"
)

// Options is the sandbox configuration; the integrator fills it from config.
type Options struct {
	Enabled  bool     // false: Mode None, why "disabled by config"
	Root     string   // the workspace root, bound read-write
	RWPaths  []string // more read-write binds (the machine-shared tier, a build cache)
	Bwrap    string   // the bwrap binary; "" looks it up on PATH (Linux only; Seatbelt is always /usr/bin/sandbox-exec)
	EnvDrop  []string // more env names to drop; `*` globs allowed (e.g. AWS_*)
	EnvAllow []string // names that survive scrubbing; wins over every drop rule
	EnvSet   []string // NAME=value set after scrubbing (the package registries); wins over the process env
	Timeout  time.Duration
}

// Call is what varies per command: whether the gate let it reach the network, and its cwd.
type Call struct {
	Network bool
	Cwd     string
}

// Probe is the reading from trying the platform's fence once.
type Probe struct {
	Mode      Mode // the fence probed: Bwrap or Seatbelt ("" reads as Bwrap)
	Available bool
	Path      string
	Version   string
	Why       string // why not, when unavailable
}

// Sandbox is the probed policy. Zero value: Mode None.
type Sandbox struct {
	opt   Options
	probe Probe
	once  sync.Once
}

// New builds a sandbox; the probe runs lazily on first use.
func New(opt Options) *Sandbox { return &Sandbox{opt: opt} }

// Options returns the configuration the sandbox was built with.
func (s *Sandbox) Options() Options {
	if s == nil {
		return Options{}
	}
	return s.opt
}

// Mode reports bwrap, seatbelt or none.
func (s *Sandbox) Mode() Mode {
	if s == nil || !s.opt.Enabled {
		return None
	}
	if p := s.Probe(); p.Available {
		if p.Mode == "" {
			return Bwrap
		}
		return p.Mode
	}
	return None
}

// Why explains a None mode; "" under a fence.
func (s *Sandbox) Why() string {
	if s == nil {
		return "no sandbox configured"
	}
	if !s.opt.Enabled {
		return "disabled by config"
	}
	return s.Probe().Why
}

// Probe tries the platform's fence once and caches the result: bwrap on Linux, Seatbelt on
// macOS, nothing elsewhere.
func (s *Sandbox) Probe() Probe {
	if s == nil {
		return Probe{Why: "no sandbox configured"}
	}
	s.once.Do(func() { s.probe = probeFor(hostOS, s.opt) })
	return s.probe
}

// The OS and the probes are variables so the choice of fence is testable on any machine.
var (
	hostOS        = runtime.GOOS
	probeBwrap    = ProbeBwrap
	probeSeatbelt = ProbeSeatbelt
)

func probeFor(goos string, opt Options) Probe {
	switch goos {
	case "linux":
		return probeBwrap(opt.Bwrap, opt.Timeout)
	case "darwin":
		return probeSeatbelt(SeatbeltBin, opt.Timeout)
	}
	return Probe{Why: "no sandbox for " + goos + " (bwrap is Linux's, Seatbelt macOS's)"}
}

// ProbeBwrap tries to run /bin/true under bwrap with the flags this package uses and reports
// whether it works, and if not, why (the kernel or container refusing namespaces is common).
func ProbeBwrap(bin string, timeout time.Duration) Probe {
	if bin == "" {
		p, err := exec.LookPath("bwrap")
		if err != nil {
			return Probe{Mode: Bwrap, Why: "bwrap not on PATH"}
		}
		bin = p
	}
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	ver, _ := exec.CommandContext(ctx, bin, "--version").Output()
	args := append(baseArgs(Options{}, Call{}), "--", "/bin/true")
	cmd := exec.CommandContext(ctx, bin, args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		why := strings.TrimSpace(string(out))
		if why == "" {
			why = err.Error()
		}
		return Probe{Mode: Bwrap, Path: bin, Version: strings.TrimSpace(string(ver)), Why: "bwrap cannot run here: " + firstLine(why)}
	}
	return Probe{Mode: Bwrap, Available: true, Path: bin, Version: strings.TrimSpace(string(ver))}
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}

func baseArgs(opt Options, call Call) []string {
	args := []string{
		"--ro-bind", "/", "/",
		"--tmpfs", "/tmp",
		"--dev", "/dev",
		"--proc", "/proc",
		"--unshare-pid", "--unshare-ipc", "--unshare-uts", // the child sees its own processes only
		"--die-with-parent",
		"--new-session",
	}
	args = append(args, maskSecrets()...)
	if opt.Root != "" {
		args = append(args, "--bind", opt.Root, opt.Root)
	}
	for _, p := range opt.RWPaths {
		if p != "" {
			if _, err := os.Stat(p); err == nil {
				args = append(args, "--bind", p, p)
			}
		}
	}
	if !call.Network {
		args = append(args, "--unshare-net")
	}
	if call.Cwd != "" {
		args = append(args, "--chdir", call.Cwd)
	}
	return args
}

// secretHomes are the home paths that hold credentials: inside the sandbox each existing one is
// masked — an empty tmpfs over a directory, /dev/null over a file — so a command reads nothing
// there even though the rest of the disk is readable.
var secretHomes = []string{
	".ssh", ".gnupg", ".aws", ".azure", ".kube", ".docker", ".netrc", ".git-credentials", ".npmrc",
	".pypirc", ".config/gcloud", ".config/gh", ".config/hub", ".password-store", ".local/share/keyrings",
	".mozilla", ".config/google-chrome", ".config/chromium", ".config/BraveSoftware", "snap/chromium",
}

func maskSecrets() []string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" || home == "/" {
		return nil
	}
	var out []string
	for _, rel := range secretHomes {
		p := path.Join(home, rel)
		fi, err := os.Stat(p)
		switch {
		case err != nil:
			continue
		case fi.IsDir():
			out = append(out, "--tmpfs", p)
		default:
			out = append(out, "--ro-bind", "/dev/null", p)
		}
	}
	return out
}

// Wrap prefixes argv with the fence's invocation for this call — bwrap's flags, or
// sandbox-exec's profile and parameters; under None it returns argv as is.
func (s *Sandbox) Wrap(call Call, argv []string) []string {
	var out []string
	switch s.Mode() {
	case Bwrap:
		out = append([]string{s.probe.Path}, baseArgs(s.opt, call)...)
	case Seatbelt:
		out = append([]string{s.probe.Path}, seatbeltArgs(s.opt, call)...)
	default:
		return argv
	}
	out = append(out, "--")
	return append(out, argv...)
}

// Command builds an exec.Cmd for argv under this sandbox with a scrubbed environment and its own
// process group (so a timeout can kill everything it started). env nil means os.Environ().
// When ctx ends the whole group is killed, and a child left holding the output pipes cannot keep
// Wait past WaitDelay.
func (s *Sandbox) Command(ctx context.Context, call Call, env []string, argv ...string) *exec.Cmd {
	if env == nil {
		env = os.Environ()
	}
	w := s.Wrap(call, argv)
	cmd := exec.CommandContext(ctx, w[0], w[1:]...)
	cmd.Env = s.Scrub(env)
	cmd.Dir = call.Cwd
	return Contain(cmd)
}

// Contain gives an exec.CommandContext command its own process group, killed whole when the
// context ends, and bounds Wait by WaitDelay so a backgrounded child holding the output pipes
// cannot outlive the deadline.
func Contain(cmd *exec.Cmd) *exec.Cmd {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		return cmd.Process.Kill()
	}
	cmd.WaitDelay = WaitDelay
	return cmd
}

// WaitDelay is how long Wait waits for the output pipes after the process is gone or killed.
var WaitDelay = 2 * time.Second

// Scrub applies this sandbox's env rules (nil-safe: the default rules alone).
func (s *Sandbox) Scrub(env []string) []string {
	if s == nil {
		return ScrubEnv(env, nil, nil)
	}
	out := ScrubEnv(env, s.opt.EnvDrop, s.opt.EnvAllow)
	if len(s.opt.EnvSet) == 0 {
		return out
	}
	set := map[string]bool{}
	for _, kv := range s.opt.EnvSet {
		k, _, _ := strings.Cut(kv, "=")
		set[k] = true
	}
	kept := out[:0]
	for _, kv := range out {
		if k, _, _ := strings.Cut(kv, "="); !set[k] {
			kept = append(kept, kv)
		}
	}
	return append(kept, s.opt.EnvSet...)
}

var (
	secretName = regexp.MustCompile(`(?i)(^|_)(API_KEY|TOKEN|SECRET|PASSWORD|PASSWD|PRIVATE_KEY|ACCESS_KEY|CREDENTIALS?)(_|$)`)
	// a URL carrying user:password, as DATABASE_URL usually does
	secretValue = regexp.MustCompile(`://[^/@\s:]*:[^/@\s]+@`)
)

// ScrubEnv drops secrets from an environment: any name holding API_KEY, TOKEN, SECRET, PASSWORD,
// PRIVATE_KEY, ACCESS_KEY or CREDENTIAL as a whole word (case-insensitive), any value that is a URL
// with a password in it, plus the caller's names (globs allowed). allow wins.
func ScrubEnv(env, drop, allow []string) []string {
	out := make([]string, 0, len(env))
	for _, kv := range env {
		name, value, _ := strings.Cut(kv, "=")
		if Dropped(name, drop, allow) || secretValue.MatchString(value) && !allowed(name, allow) {
			continue
		}
		out = append(out, kv)
	}
	return out
}

// Dropped reports whether an env name is scrubbed under the rules.
func Dropped(name string, drop, allow []string) bool {
	if allowed(name, allow) {
		return false
	}
	if secretName.MatchString(name) {
		return true
	}
	for _, d := range drop {
		if match(d, name) {
			return true
		}
	}
	return false
}

func allowed(name string, allow []string) bool {
	for _, a := range allow {
		if match(a, name) {
			return true
		}
	}
	return false
}

func match(pattern, name string) bool {
	if pattern == name {
		return true
	}
	ok, err := path.Match(pattern, name)
	return err == nil && ok
}

// Status renders the sandbox for the status board.
func (s *Sandbox) Status() string {
	switch s.Mode() {
	case Bwrap:
		return fmt.Sprintf("sandbox: bwrap (%s)", s.probe.Version)
	case Seatbelt:
		return "sandbox: seatbelt (sandbox-exec)"
	}
	return "sandbox: none — " + s.Why()
}
