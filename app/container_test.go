package app

import (
	"strings"
	"testing"

	"github.com/Kaginari/agent-one/config"
	"github.com/Kaginari/agent-one/tool"
)

func TestContainerArgsMountTheWorldReadWriteAndTheRestReadOnly(t *testing.T) {
	s := ContainerSpec{Dist: "agent-one", Image: "agent-one-runtime:x", Exe: "/opt/agent-one", Root: "/w", Cwd: "/w/src", Home: "/home/u", UID: 1000, GID: 1000,
		RO: []string{"/home/u/.config/agent-one"}, RW: []string{"/home/u/.local/share/agent-one"}, EnvKeys: []string{"OPENROUTER_API_KEY", "ARTIFACTORY_TOKEN"},
		EnvSet: []string{"NPM_CONFIG_REGISTRY=https://art/npm/"}, Argv: []string{"run", "hi"}}
	got := strings.Join(s.Args(), " ")
	for _, want := range []string{
		"-v /w:/w ", "-v /opt/agent-one:/usr/local/bin/agent-one:ro", "-v /home/u/.config/agent-one:/home/u/.config/agent-one:ro",
		"-v /home/u/.local/share/agent-one:/home/u/.local/share/agent-one ", "--user 1000:1000", "-e OPENROUTER_API_KEY ",
		"-e AGENT_ONE_DOCKER_AGENT=1", "-e ARTIFACTORY_TOKEN ", "-e NPM_CONFIG_REGISTRY=https://art/npm/", "-w /w/src agent-one-runtime:x /usr/local/bin/agent-one run hi", "--network host",
	} {
		if !strings.Contains(got+" ", want) {
			t.Fatalf("docker argv misses %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "=sk-") || strings.Contains(got, " -t ") {
		t.Fatalf("a key value or a TTY without one: %s", got)
	}
}

func TestContainerFlagsAndEnv(t *testing.T) {
	rest, on, img, _ := stripContainerFlags([]string{"--docker-agent", "--image", "my:img", "run", "x", "--image=ignored-later"})
	if !on || img != "ignored-later" || strings.Join(rest, " ") != "run x" {
		t.Fatalf("strip: %v %v %q", rest, on, img)
	}
	keys := strings.Join(passEnv("agent-one", []string{"PATH=/bin", "HOME=/h", "OPENROUTER_API_KEY=k", "AGENT_ONE_THEME=light", "AGENT_ONE_DOCKER_AGENT=1", "TERM=xterm", "AWS_SECRET=s"}), " ")
	if keys != "AGENT_ONE_THEME OPENROUTER_API_KEY TERM" {
		t.Fatalf("passed env %q", keys)
	}
	if !strings.HasPrefix(DefaultRuntimeImage("agent-one"), "agent-one-runtime:") {
		t.Fatal(DefaultRuntimeImage("agent-one"))
	}
}

func TestContaineredIsANoOpInside(t *testing.T) {
	io := IO{Env: func(k string) string {
		if k == "AGENT_ONE_DOCKER_AGENT" {
			return "1"
		}
		return ""
	}}
	if _, handled := DockerAgent("agent-one", []string{"--docker-agent", "status"}, io); handled {
		t.Fatal("inside the container the flag must not start another one")
	}
}

// policy.humanGate.trustContainer: inside the harness's own runtime container the sandbox is the
// membrane — every class pre-approved at the Gate; outside, or with the knob off, outward and
// destructive still ask. (The guard denylist sits one step earlier and is untouched.)
func TestTrustContainerPreapprovesInsideOnly(t *testing.T) {
	cfg := &config.Config{}
	cfg.Dist.Name = "agent-one"
	cfg.Policy.HumanGate.TrustContainer = true
	envIn := func(k string) string {
		if k == "AGENT_ONE_DOCKER_AGENT" {
			return "1"
		}
		return ""
	}
	envOut := func(string) string { return "" }
	inside := (&App{Cfg: cfg, Opt: Options{Env: envIn}}).newGate()
	for _, cl := range []tool.Class{tool.Read, tool.Write, tool.Outward, tool.Destructive} {
		if !inside.Approve[cl] {
			t.Fatalf("inside the container every class pre-approves; %s still asks", cl)
		}
	}
	for name, a := range map[string]*App{
		"outside":       {Cfg: cfg, Opt: Options{Env: envOut}},
		"knob off":      {Cfg: &config.Config{}, Opt: Options{Env: envIn}},
		"no env reader": {Cfg: cfg, Opt: Options{}},
	} {
		g := a.newGate()
		if g.Approve[tool.Outward] || g.Approve[tool.Destructive] {
			t.Fatalf("%s: outward/destructive must still ask (trustContainer only inside the sandbox)", name)
		}
	}
}

func TestSetupYAMLLoads(t *testing.T) {
	for name, c := range setupPresets {
		if c.model == "openai/" || c.model == "ollama/" || c.model == "vllm/" {
			c.model += "some-model"
		}
		y := setupYAML(c, 7411)
		if !strings.Contains(y, "default:") || !strings.Contains(y, c.provider+":") {
			t.Fatalf("%s: %s", name, y)
		}
		if c.keyEnv != "" && !strings.Contains(y, "apiKeyEnv: "+c.keyEnv) {
			t.Fatalf("%s: the key's name is missing: %s", name, y)
		}
	}
}

func TestSetupConfigOpensTheWorld(t *testing.T) {
	for name, c := range setupPresets {
		if strings.HasSuffix(c.model, "/") {
			c.model += "some-model"
		}
		w := newTestWorkspace(t, "agent-one", agentOneMembers())
		w.write(".agent-one/config.yaml", setupYAML(c, 7411))
		a := w.open()
		if got := a.Cfg.Models.Default.Model; got != c.model {
			t.Fatalf("%s: the workspace runs on %q, want %q", name, got, c.model)
		}
		if p := a.Cfg.Providers[c.provider]; p == nil || !p.Enabled {
			t.Fatalf("%s: provider %s not enabled", name, c.provider)
		}
	}
}

func TestGuardPatternsFromConfig(t *testing.T) {
	w := newTestWorkspace(t, "agent-one", agentOneMembers())
	w.write(".agent-one/guards.yaml", "- '(^|[[:space:]])terraform[[:space:]]+destroy'\n")
	a := w.open()
	g := a.theGuard()
	if r := g.Match("terraform destroy -auto-approve"); r == nil || !strings.Contains(r.Source, "guards.yaml") {
		t.Fatalf("a pattern in guards.yaml blocks, named by its file: %+v", r)
	}
	if g.Match("rm -rf ~") == nil {
		t.Fatal("the built-in list stays")
	}
	if !strings.Contains(a.guardLine(), "3 sources") {
		t.Fatalf("status counts the config as a source: %s", a.guardLine())
	}
}

func TestRegistryToolsReachTheToolbox(t *testing.T) {
	w := newTestWorkspace(t, "agent-one", agentOneMembers())
	w.write(".agent-one/registry.yaml", "tools:\n  - {name: kubectl-corp, description: the cluster CLI behind the corp proxy, triggers: [kube]}\n")
	a := w.open()
	tb, err := a.Workspace.Toolbox()
	if err != nil {
		t.Fatal(err)
	}
	reg, err := tb.BuildRegistry(false)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range reg.Entries {
		if e.Name == "kubectl-corp" {
			return
		}
	}
	t.Fatal("a registry.tools entry is not in the toolbox")
}

// The flag's former name still runs the agent, and says what it is called now.
func TestContaineredIsTheDockerAgentsFormerName(t *testing.T) {
	_, on, _, legacy := stripContainerFlags([]string{"--containered", "run", "x"})
	if !on || !legacy {
		t.Fatalf("--containered must still turn the docker agent on (on=%v legacy=%v)", on, legacy)
	}
	_, on, _, legacy = stripContainerFlags([]string{"--docker-agent", "run", "x"})
	if !on || legacy {
		t.Fatalf("--docker-agent is the flag (on=%v legacy=%v)", on, legacy)
	}
}
