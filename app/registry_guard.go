package app

import (
	"encoding/json"
	"regexp"
	"strings"

	"github.com/Kaginari/agent-one/config"
	"github.com/Kaginari/agent-one/guard"
	"github.com/Kaginari/agent-one/loop"
)

// registry.packages.policy: all (the default) lets a command fetch packages from anywhere;
// private-only keeps every package manager on the configured private registry. Under
// private-only a bash command is refused, before any approval, when it
//   - points a manager at another registry: npm/yarn/pnpm --registry, pip/uv --index-url, -i,
//     --extra-index-url, --find-links, --index, a GOPROXY / GONOPROXY / GOPRIVATE / GOFLAGS
//     assignment or `go env -w` of them, or `<manager> config set registry …` — unless the
//     value is the configured private URL;
//   - installs from a URL or a VCS (pip install git+https://…, npm install https://…);
//   - installs with a manager that has no private URL configured (nowhere private to go);
//   - fetches from a public registry host with curl or wget.
// The environment does its part too (Packages.PolicyEnv): GOPROXY keeps only the private
// proxy (no `direct`), and a manager with no private URL is switched off (GOPROXY=off,
// PIP_NO_INDEX=1).

var (
	// cmdAt is a command position: the line's start or after a separator, past wrappers and
	// variable assignments (`sudo`, `env`, `GOFLAGS=x`) — an argument (`echo npm i`) is not one
	cmdAt       = `(^|[;&|(` + "`" + `]|\$\()[[:space:]]*((sudo|env|time|nice|nohup|command|exec)[[:space:]]+|[A-Za-z_][A-Za-z0-9_]*=("[^"]*"|'[^']*'|[^[:space:]]*)[[:space:]]+)*`
	npmish      = `(npm|npx|pnpm|pnpx|yarn|bun|bunx)`
	pipish      = `(pip|pip3|pipx|uv|poetry|pdm|python3?[[:space:]]+-m[[:space:]]+pip)`
	flagValue   = `(=|[[:space:]]+)("[^"]*"|'[^']*'|[^[:space:]]+)`
	reNpmFlag   = regexp.MustCompile(cmdAt + npmish + `[[:space:]][^;&|]*--registry` + flagValue)
	rePipFlag   = regexp.MustCompile(cmdAt + pipish + `[[:space:]][^;&|]*(--index-url|--extra-index-url|--find-links|--default-index|--index|[[:space:]]-i|[[:space:]]-f)` + flagValue)
	reGoVar     = regexp.MustCompile(`(^|[;&|(` + "`" + `[:space:]]|-w[[:space:]]+)(GOPROXY|GONOPROXY|GOPRIVATE|GOFLAGS|GOINSECURE)=("[^"]*"|'[^']*'|[^[:space:]]*)`)
	reCfgSet    = regexp.MustCompile(cmdAt + `(` + npmish + `|` + pipish + `)[[:space:]]+config[[:space:]]+set[[:space:]]+([^[:space:]]*(registry|index-url|index_url|extra-index-url))`)
	reURLSource = regexp.MustCompile(cmdAt + `(` + npmish + `|` + pipish + `)[[:space:]]+(install|i|add|ci|download)[[:space:]][^;&|]*(git\+|https?://|git@|github:)`)
	rePublic    = regexp.MustCompile(cmdAt + `(curl|wget)[[:space:]][^;&|]*(registry\.npmjs\.org|registry\.yarnpkg\.com|pypi\.org|files\.pythonhosted\.org|proxy\.golang\.org)`)
	reNpmUse    = regexp.MustCompile(cmdAt + `(` + npmish + `[[:space:]]+(install|i|add|ci|update|upgrade|dlx|exec|create)([[:space:]]|$)|(npx|pnpx|bunx)[[:space:]])`)
	rePipUse    = regexp.MustCompile(cmdAt + pipish + `[[:space:]]+(install|download|add|sync|lock|pip[[:space:]]+install|tool[[:space:]]+install|run)([[:space:]]|$)`)
	reGoUse     = regexp.MustCompile(cmdAt + `go[[:space:]]+(get|install|mod[[:space:]]+(download|tidy))([[:space:]]|$)`)
)

// registryRefusal is why a command breaks the private-only policy, or "".
func registryRefusal(p config.Packages, cmd string) string {
	if !p.PrivateOnly() {
		return ""
	}
	cmd = guard.CommandWords(cmd)
	unquote := func(v string) string { return strings.Trim(v, `"'`) }
	same := func(v, private string) bool {
		v, private = strings.TrimSuffix(unquote(v), "/"), strings.TrimSuffix(private, "/")
		return private != "" && v == private
	}
	refuse := func(what string) string {
		return "blocked by registry.packages.policy: private-only — " + what + ". Packages come only from the configured private registry" +
			p.PrivateList() + ". Do not retry or work around it; use that registry, or tell the human what you need."
	}
	for _, m := range reNpmFlag.FindAllStringSubmatch(cmd, -1) {
		if !same(m[len(m)-1], p.Npm) {
			return refuse("another npm registry (" + unquote(m[len(m)-1]) + ")")
		}
	}
	for _, m := range rePipFlag.FindAllStringSubmatch(cmd, -1) {
		if !same(m[len(m)-1], p.Pip) {
			return refuse("another Python index (" + unquote(m[len(m)-1]) + ")")
		}
	}
	for _, m := range reGoVar.FindAllStringSubmatch(cmd, -1) {
		v := unquote(m[3])
		if m[2] == "GOPROXY" && same(v, p.PrivateGoProxy()) || m[2] == "GOFLAGS" && !strings.Contains(v, "insecure") {
			continue
		}
		return refuse("a " + m[2] + " override")
	}
	if m := reCfgSet.FindStringSubmatch(cmd); m != nil {
		return refuse("re-pointing a package manager's registry (" + strings.TrimSpace(m[0]) + ")")
	}
	// the private URL given as a flag value is no URL install: strip the flag values first
	bare := reNpmFlag.ReplaceAllString(rePipFlag.ReplaceAllString(cmd, " "), " ")
	if m := reURLSource.FindStringSubmatch(bare); m != nil {
		return refuse("an install straight from a URL or a repository")
	}
	if m := rePublic.FindStringSubmatch(cmd); m != nil {
		return refuse("a download from a public registry (" + m[len(m)-1] + ")")
	}
	if p.Npm == "" && reNpmUse.MatchString(cmd) {
		return refuse("no private npm registry is configured (registry.packages.npm)")
	}
	if p.Pip == "" && rePipUse.MatchString(cmd) {
		return refuse("no private Python index is configured (registry.packages.pip)")
	}
	if p.PrivateGoProxy() == "" && reGoUse.MatchString(cmd) {
		return refuse("no private Go proxy is configured (registry.packages.go)")
	}
	return ""
}

// registryGuardHook refuses a bash command that breaks the private-only policy; nil when the
// policy is all.
func (a *App) registryGuardHook() func(s *loop.Session, tool string, input json.RawMessage) string {
	p := a.Cfg.Registry.Packages
	if !p.PrivateOnly() {
		return nil
	}
	return func(_ *loop.Session, name string, input json.RawMessage) string {
		if name != "bash" {
			return ""
		}
		var in struct {
			Command string `json:"command"`
		}
		if json.Unmarshal(input, &in) != nil {
			return ""
		}
		return registryRefusal(p, in.Command)
	}
}
