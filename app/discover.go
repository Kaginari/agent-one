package app

import (
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/Kaginari/agent-one/config"
	"github.com/Kaginari/agent-one/discover"
)

// Discovered is what the other harnesses and the workspace's own dirs hold: Skills (level 1:
// name and description), slash commands and Agents (agent definitions). Each source is behind
// its config switch; the first name found wins in config path order.
type Discovered struct {
	Skills   []discover.Skill
	Commands []discover.Command
	Agents   []discover.Agent
	Holes    []string
}

// Discover reads discovery.skills/commands/agents. The Claude Code and OpenCode dirs go
// through the discover package; the native and extra dirs config lists are read the same way.
func Discover(cfg *config.Config, root, home string) Discovered {
	var d Discovered
	claude, oc := cfg.Discovery.Skills.Enabled, cfg.Discovery.Skills.Enabled
	opt := discover.Options{Root: root, Home: home, WorkspaceDir: cfg.Dist.WorkspaceDir, Skills: cfg.Discovery.Skills.Enabled, Commands: cfg.Discovery.Commands.Enabled, Agents: cfg.Discovery.Agents.Enabled, Sources: discover.Sources{Claude: &claude, OpenCode: &oc}}
	if cfg.Discovery.Skills.Enabled {
		// one registry, one precedence (discover.SkillBases): crafted > base > project host >
		// global host, then the config's extra paths, then the binary's builtins. A name with two
		// homes is a hole naming both, never a silent shadow.
		idx := discover.NewSkillIndex()
		covered := map[string]bool{}
		for _, dir := range opt.SkillDirs() {
			covered[filepath.Clean(dir)] = true
		}
		for _, ds := range skillDirSources(opt) {
			idx.ReadDir(ds[0], ds[1])
		}
		for _, dir := range expandPaths(cfg.Discovery.Skills.Paths, root, home) {
			if covered[filepath.Clean(dir)] {
				continue
			}
			idx.ReadDir(dir, "native")
		}
		// the Skills the binary ships, unless the workspace holds its own of the same name
		for _, b := range builtinSkills(cfg.Dist) {
			idx.Add(b)
		}
		d.Skills = idx.Skills()
		d.Holes = append(d.Holes, idx.Holes...)
	}
	if cfg.Discovery.Commands.Enabled {
		d.Commands = discover.Commands(opt)
		seen := map[string]bool{}
		for _, c := range d.Commands {
			seen[c.Name] = true
		}
		for _, dir := range expandPaths(cfg.Discovery.Commands.Paths, root, home) {
			if known(dir) {
				continue
			}
			_ = filepath.Walk(dir, func(p string, info os.FileInfo, err error) error {
				if err != nil || info.IsDir() || !strings.HasSuffix(p, ".md") {
					return nil
				}
				rel, _ := filepath.Rel(dir, p)
				name := strings.ReplaceAll(strings.TrimSuffix(filepath.ToSlash(rel), ".md"), "/", ":")
				if seen[name] {
					return nil
				}
				data, err := os.ReadFile(p)
				if err != nil {
					return nil
				}
				fm, agent := discover.Frontmatter(string(data))
				seen[name] = true
				d.Commands = append(d.Commands, discover.Command{Name: name, Description: fm["description"], Path: p, Source: "native", Frontmatter: fm, Template: agent})
				return nil
			})
		}
		sort.SliceStable(d.Commands, func(i, j int) bool { return d.Commands[i].Name < d.Commands[j].Name })
	}
	if cfg.Discovery.Agents.Enabled {
		d.Agents = discover.Agents(opt)
	}
	return d
}

// skillDirSources pairs each dir discover reads with its source label, in precedence order.
func skillDirSources(opt discover.Options) [][2]string {
	dirs := opt.SkillDirs()
	labels := discover.SkillSources(opt)
	out := make([][2]string, len(dirs))
	for i := range dirs {
		out[i] = [2]string{dirs[i], labels[i]}
	}
	return out
}

// known says whether the discover package already reads a command or agent dir (its fixed
// CC/OC set); skills go through discover.SkillBases instead.
func known(dir string) bool {
	d := filepath.ToSlash(dir)
	for _, suffix := range []string{"/.claude/commands", "/.opencode/command", "/.opencode/commands", "/.config/opencode/command", "/.config/opencode/commands",
		"/.claude/agents", "/.opencode/agent", "/.opencode/agents", "/.config/opencode/agent", "/.config/opencode/agents"} {
		if strings.HasSuffix(d, suffix) {
			return true
		}
	}
	return false
}

// expandPaths resolves config's discovery paths: `~/` to home, relative to the root; a
// `{a,b}` brace group expands to one path each.
func expandPaths(paths []string, root, home string) []string {
	var out []string
	for _, p := range paths {
		for _, q := range braces(p) {
			switch {
			case strings.HasPrefix(q, "~/"):
				q = filepath.Join(home, q[2:])
			case !filepath.IsAbs(q):
				q = filepath.Join(root, q)
			}
			out = append(out, q)
		}
	}
	return out
}

func braces(p string) []string {
	i := strings.IndexByte(p, '{')
	j := strings.IndexByte(p, '}')
	if i < 0 || j < i {
		return []string{p}
	}
	var out []string
	for _, alt := range strings.Split(p[i+1:j], ",") {
		out = append(out, braces(p[:i]+alt+p[j+1:])...)
	}
	return out
}

// Skill finds a Skill by name.
func (d Discovered) Skill(name string) (discover.Skill, bool) {
	for _, s := range d.Skills {
		if strings.EqualFold(s.Name, name) {
			return s, true
		}
	}
	return discover.Skill{}, false
}

// Command finds a slash command by name.
func (d Discovered) Command(name string) (discover.Command, bool) {
	for _, c := range d.Commands {
		if strings.EqualFold(c.Name, name) {
			return c, true
		}
	}
	return discover.Command{}, false
}
