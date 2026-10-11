package app

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/Kaginari/agent-one/loop"
	"github.com/Kaginari/agent-one/tool"
)

// tmpModes are the two block modes the config surface accepts.
var tmpModes = map[string]bool{"rewrite": true, "refuse": true}

func tmpMode(s string) string {
	if tmpModes[s] {
		return s
	}
	return "rewrite"
}

// ScratchDir is the workspace's scratch root. nil workspace → "" empty → no scratch redirect.
func (a *App) ScratchDir() string {
	var wd string
	if a.Workspace != nil {
		wd = a.Workspace.Lex.WorkspaceDir
	}
	if wd == "" {
		wd = tool.DefaultWorkspaceDir
	}
	return wd + "/tmp"
}

// agentScratch namespaces a fresh redirect under the acting agent's run so one run's litter never
// collides with a sibling's. Shared tools staged under tmp/ (gosdk, ts-bin, gocache) stay
// workspace-level — an attempt that writes through them keeps the shared dir.
func (a *App) agentScratch(s *loop.Session) string {
	if s == nil || s.RunID == "" {
		return a.ScratchDir()
	}
	return a.ScratchDir() + "/scratch/" + s.RunID
}

// AgentRoot is the workspace root; every absolute redirection resolves under it.
func (a *App) AgentRoot() string { return a.Root }

// absScratch is a scratch dir in absolute form — used for bash words, which must survive a
// `cd:` override (a relative path there would land outside the workspace).
func (a *App) absScratch(scratch string) string {
	if strings.HasPrefix(a.Root, "/") && !strings.HasPrefix(scratch, "/") {
		return strings.TrimSuffix(a.Root, "/") + "/" + scratch
	}
	return scratch
}

// tmpTarget rewrites an absolute path into the agent's scratch dir, preserving the suffix:
//
//	/tmp/x        → .agent-one/tmp/scratch/<run>/x
//	/var/log/l    → .agent-one/tmp/scratch/<run>/system/var/log/l
//	/outside/f    → .agent-one/tmp/scratch/<run>/root/outside/f
//	a/relative    → "" (untouched — already inside the workspace's own relative space)
//
// paths that carry one of the shared tmp tools (gosdk, ts-bin, gocache) keep the shared dir
func (a *App) tmpTarget(scratch, path string) string {
	if scratch == "" || path == "" || !strings.HasPrefix(path, "/") {
		return path
	}
	root := strings.TrimSuffix(a.AgentRoot(), "/")
	if root != "" && (path == root || strings.HasPrefix(path, root+"/")) {
		return path // already inside the workspace — never nest
	}
	rest := strings.TrimPrefix(path, "/")
	if strings.HasPrefix(rest, "tmp/") || rest == "tmp" {
		return scratch + "/" + strings.TrimPrefix(strings.TrimPrefix(rest, "tmp"), "/")
	}
	switch {
	case strings.HasPrefix(rest, "private/"):
		return scratch + "/system/" + rest
	case strings.HasPrefix(rest, "var/"), strings.HasPrefix(rest, "etc/"), strings.HasPrefix(rest, "usr/"),
		strings.HasPrefix(rest, "opt/"), strings.HasPrefix(rest, "sbin/"), strings.HasPrefix(rest, "bin/"),
		strings.HasPrefix(rest, "lib/"), strings.HasPrefix(rest, "cores/"), strings.HasPrefix(rest, "dev/"):
		return scratch + "/system/" + rest
	default:
		return scratch + "/root/" + rest
	}
}

// probeVerbs are the file-inspection verbs: a command made only of these with path words is
// existence-reading. Rewriting those into the agent's scratch misreports the machine (scratch is
// never pre-staged) and has produced a false "not downloaded → blocked" escalation in-session.
// The rewrite message in that case gets a marker so the model reads the shape, not a lie.
var probeVerbs = map[string]bool{
	"ls": true, "cat": true, "head": true, "tail": true, "stat": true, "wc": true,
	"file": true, "readlink": true, "test": true, "[": true, "[[": true,
}

// isProbe says the command is read-only inspection: every command word is a probe verb or a
// path-ish argument, and nothing writes (no redirects, no pipes to file, no mutation verbs).
func isProbe(cmd string) bool {
	words := strings.Fields(cmd)
	if len(words) == 0 {
		return false
	}
	sawVerb := false
	for _, w := range words {
		if strings.ContainsAny(w, "<>|;") {
			return false // pipes/redirects — analysis would need real parsing, step back
		}
		// VAR=value assignments ride along (neutral)
		if strings.Contains(w, "=") && !strings.HasPrefix(w, "=") {
			continue
		}
		lw := strings.ToLower(w)
		if probeVerbs[lw] {
			sawVerb = true
			continue
		}
		// flags and path-ish words ride along
		if strings.HasPrefix(w, "-") || strings.Contains(w, "/") {
			continue
		}
		// a bare non-verb word (echo X): echo is read-only
		if lw == "echo" || lw == "printf" {
			sawVerb = true
			continue
		}
		return false
	}
	return sawVerb
}

// probeMarker appends the honesty note a redirected probe owes the model: nothing is pre-staged
// in your scratch dir; the machine path you asked about was mapped, not copied.
func probeMarker(cmd string) string {
	if !isProbe(cmd) {
		return ""
	}
	return "\n(probe-redirect: existence-reading was remapped into your scratch — scratch is never pre-staged, so 'missing' there says nothing about the machine; ask the human or cite the workspace's sanctioned in-workspace path instead)"
}

// absolute scratch path (bash words must survive `cwd:`), and reports the rewrites. A word
// qualifies when it is exactly a path or a VAR=path assignment; `cd` / `chdir` into off-workspace
// is refused outright under refuseForCwd.
func rewriteTmpCommand(cmd, scratch, workspace string, refuseForCwd bool) (out, msg string) {
	if scratch == "" || !strings.Contains(cmd, "/") {
		return cmd, ""
	}
	words := strings.Fields(cmd)
	var rewrites []string
	for i, w := range words {
		if w == "--" {
			break
		}
		lower := strings.ToLower(w)
		// never rewrite a target already inside scratch
		if strings.Contains(w, scratch) {
			continue
		}
		// chdir family: refuse outright when refuseForCwd, else rewrite the operand
		isCd := lower == "cd" || lower == "chdir"
		if isCd && refuseForCwd {
			if i+1 < len(words) {
				return cmd, fmt.Sprintf("cd %s", words[i+1])
			}
		}
		if i > 0 && (strings.EqualFold(words[i-1], "cd") || strings.EqualFold(words[i-1], "chdir")) && refuseForCwd && strings.HasPrefix(w, "/") {
			return cmd, "cd " + w
		}
		name, val, assign := strings.Cut(w, "=")
		if !assign || name == "" || strings.ContainsAny(name, " \t") {
			name, val, assign = w, w, false
		}
		if !strings.HasPrefix(val, "/") || strings.HasPrefix(val, "//") {
			continue
		}
		// a path inside the workspace is the workspace's own, wherever the workspace lives (/app in a
		// container, a dir under /tmp or /var): only what leaves it is herded into scratch
		if workspace != "" && (strings.TrimRight(val, "/;&|)'\"") == workspace || strings.HasPrefix(val, workspace+"/")) {
			continue
		}
		root, rest := classifyRoot(val)
		if root == "" {
			continue
		}
		var to string
		switch root {
		case "/":
			to = scratch + "/root/" + rest
		case "/tmp":
			to = scratch
			if rest != "" {
				to = scratch + "/" + rest
			}
		case "/private", "/var", "/etc", "/usr", "/opt", "/sbin", "/bin", "/lib", "/cores", "/dev":
			to = scratch + "/system" + root + "/" + rest
		}
		if assign {
			words[i] = name + "=" + to
		} else {
			words[i] = to
		}
		rewrites = append(rewrites, w+" -> "+to)
	}
	if len(rewrites) == 0 {
		return cmd, ""
	}
	return strings.Join(words, " "), "tmp-redirect: " + strings.Join(rewrites, ", ")
}

// classifyRoot decides which known off-workspace root a path hangs from and the suffix beneath it.
func classifyRoot(p string) (root, rest string) {
	if p == "/tmp" || strings.HasPrefix(p, "/tmp/") {
		return "/tmp", strings.TrimPrefix(strings.TrimPrefix(p, "/tmp"), "/")
	}
	for _, r := range []string{"/private", "/var", "/etc", "/usr", "/opt", "/sbin", "/bin", "/lib", "/cores", "/dev"} {
		if p == r || strings.HasPrefix(p, r+"/") {
			return r, strings.TrimPrefix(strings.TrimPrefix(p, r), "/")
		}
	}
	if strings.HasPrefix(p, "/") && !strings.HasPrefix(p, "//") {
		return "/", strings.TrimPrefix(p, "/")
	}
	return "", ""
}

// tmpGuardHook is the Guard hook for scratch discipline: a bash command's absolute off-workspace
// words are rewritten into the acting agent's `.agent-one/tmp/scratch/<run>` (mode rewrite) or
// refused (mode refuse); the write/edit family is refused and told to land under its own
// scratch dir. The refusal names the target so the model can correct itself.
func (a *App) tmpGuardHook() func(s *loop.Session, tool string, input json.RawMessage) string {
	if !a.Cfg.Tools.TmpRedirect.Enabled {
		return nil
	}
	mode := tmpMode(a.Cfg.Tools.TmpRedirect.Mode)
	refuseCwd := a.Cfg.Tools.TmpRedirect.RefuseCwd
	write := map[string]bool{
		"write": true, "edit": true, "multiedit": true, "patch": true,
		"str_replace_based_edit_tool": true,
	}
	return func(s *loop.Session, name string, input json.RawMessage) string {
		scratch := a.agentScratch(s)
		if name == "bash" {
			var in struct {
				Command string `json:"command"`
				Cwd     string `json:"cwd"`
			}
			if json.Unmarshal(input, &in) != nil {
				return ""
			}
			if refuseCwd && strings.HasPrefix(strings.TrimSpace(in.Cwd), "/") && !strings.HasPrefix(strings.TrimSpace(in.Cwd), a.AgentRoot()+"/") && strings.TrimSpace(in.Cwd) != a.AgentRoot() {
				return "refused: cwd " + in.Cwd + " is outside the workspace — run under the workspace's root or drop `cwd` and the command runs from " + a.AgentRoot()
			}
			if rw, msg := rewriteTmpCommand(in.Command, a.absScratch(scratch), a.AgentRoot(), refuseCwd); msg != "" {
				if strings.HasPrefix(msg, "cd ") {
					return fmt.Sprintf("refused: %s leaves the workspace — run from the workspace's root or pass no `cwd`; under mode %s a chdir is never rewritten", msg, mode)
				}
				if mode == "refuse" {
					return "refused: " + msg + probeMarker(in.Command) + " — land scratch inside your run's dir at " + scratch + " (turn `tools.tmpRedirect.mode: rewrite` on to have the guard rewrite instead)"
				}
				return "rewrite: " + msg + probeMarker(in.Command) + "\nrun this instead:\n" + rw
			}
			return ""
		}
		if write[name] {
			var in struct {
				Path string `json:"path"`
			}
			if json.Unmarshal(input, &in) != nil {
				return ""
			}
			if in.Path == "" {
				var alt struct {
					Path string `json:"file_path"`
				}
				if json.Unmarshal(input, &alt) == nil {
					in.Path = alt.Path
				}
			}
			to := a.tmpTarget(scratch, in.Path)
			if to != in.Path {
				return fmt.Sprintf("refused: %s is outside the workspace — land it at %s (Policy: scratch lives under your run's dir %s)", in.Path, to, scratch)
			}
		}
		return ""
	}
}
