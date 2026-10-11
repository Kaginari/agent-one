package app

import (
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Kaginari/agent-one/config"
	"github.com/Kaginari/agent-one/sandbox"
)

// newSandbox builds the bash sandbox from config: the platform's fence (bwrap on Linux, Seatbelt
// on macOS) unless tools.bash.sandbox is none, the workspace root and the machine-shared tier
// read-write, every provider's key variable dropped from the child environment, envAllow honoured.
func newSandbox(cfg *config.Config, root string) *sandbox.Sandbox {
	var drop []string
	for _, name := range cfg.ProviderNames() {
		if p := cfg.Providers[name]; p != nil && p.APIKeyEnv != "" {
			drop = append(drop, p.APIKeyEnv)
		}
	}
	for _, s := range cfg.MCP.Servers {
		for _, v := range s.HeadersEnv {
			drop = append(drop, v)
		}
	}
	rw := []string{cfg.Dist.MachineDir, cfg.Dist.DataDir}
	if d := cfg.Memory.Shared.Machine.Path; d != "" {
		rw = append(rw, filepath.Dir(filepath.Dir(d)))
	}
	// the package registries: their URLs, and the logins written where each manager reads one
	auth, holes := registryAuth(cfg, root, os.Getenv)
	set := overrideEnv(cfg.Registry.Packages.PackageEnv(), auth)
	cfg.Holes = append(cfg.Holes, holes...)
	return sandbox.New(sandbox.Options{
		Enabled:  cfg.Tools.Bash.Sandbox != "none",
		Root:     root,
		RWPaths:  rw,
		EnvDrop:  drop,
		EnvAllow: envAllow(cfg),
		EnvSet:   set,
		Timeout:  5 * time.Second,
	})
}

// sandboxLine is the status line for the sandbox: the mode, and why when it is none.
func sandboxLine(sb *sandbox.Sandbox) string {
	switch sb.Mode() {
	case sandbox.Bwrap:
		p := sb.Probe()
		return "sandbox: bwrap (" + strings.TrimSpace(p.Path+" "+p.Version) + ")"
	case sandbox.Seatbelt:
		return sb.Status()
	}
	return "@? sandbox: none — " + sb.Why()
}

// envAllow is bash's allow-list plus every variable the package registries' logins name.
func envAllow(cfg *config.Config) []string {
	return append(append([]string(nil), cfg.Tools.Bash.EnvAllow...), cfg.Registry.Packages.AuthEnv()...)
}

// overrideEnv is base with every KEY=value of over replacing base's KEY (or added).
func overrideEnv(base, over []string) []string {
	idx := map[string]int{}
	out := append([]string(nil), base...)
	for i, kv := range out {
		k, _, _ := strings.Cut(kv, "=")
		idx[k] = i
	}
	for _, kv := range over {
		k, _, _ := strings.Cut(kv, "=")
		if i, ok := idx[k]; ok {
			out[i] = kv
			continue
		}
		idx[k] = len(out)
		out = append(out, kv)
	}
	return out
}
