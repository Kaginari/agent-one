package app

import (
	"path/filepath"
	"strings"
	"time"

	"github.com/Kaginari/agent-one/config"
	"github.com/Kaginari/agent-one/sandbox"
)

// newSandbox builds the bash sandbox from config: bwrap unless tools.bash.sandbox is none, the
// workspace root and the machine-shared tier read-write, every provider's key variable dropped
// from the child environment, envAllow honoured.
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
	return sandbox.New(sandbox.Options{
		Enabled:  cfg.Tools.Bash.Sandbox != "none",
		Root:     root,
		RWPaths:  rw,
		EnvDrop:  drop,
		EnvAllow: envAllow(cfg),
		EnvSet:   cfg.Registry.Packages.PackageEnv(),
		Timeout:  5 * time.Second,
	})
}

// sandboxLine is the status line for the sandbox: the mode, and why when it is none.
func sandboxLine(sb *sandbox.Sandbox) string {
	if sb.Mode() == sandbox.Bwrap {
		p := sb.Probe()
		return "sandbox: bwrap (" + strings.TrimSpace(p.Path+" "+p.Version) + ")"
	}
	return "@? sandbox: none — " + sb.Why()
}

// envAllow is bash's allow-list plus the package registry's token, when one is named.
func envAllow(cfg *config.Config) []string {
	out := append([]string(nil), cfg.Tools.Bash.EnvAllow...)
	if t := cfg.Registry.Packages.TokenEnv; t != "" {
		out = append(out, t)
	}
	return out
}
