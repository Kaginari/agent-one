package app

import (
	"strings"
	"testing"

	"github.com/Kaginari/agent-one/config"
)

// private-only keeps every manager on the private registry: another registry, a URL install, a
// public host, a manager with no private URL and a proxy override are refused; the private
// registry itself, and everything under the all policy, pass.
func TestRegistryPrivateOnly(t *testing.T) {
	p := config.Packages{Policy: "private-only", Npm: "https://art.example/npm/", Pip: "https://art.example/pypi/simple", Go: "https://art.example/go,direct"}
	refused := []string{
		"npm install left-pad --registry https://registry.npmjs.org",
		"cd web && pnpm add react --registry=https://registry.npmjs.org/",
		"pip install requests -i https://pypi.org/simple",
		"pip3 install --extra-index-url https://pypi.org/simple flask",
		"python -m pip install --index-url=https://pypi.org/simple x",
		"uv pip install --index-url https://pypi.org/simple x",
		"GOPROXY=https://proxy.golang.org,direct go get example.com/m",
		"go env -w GOPROXY=direct",
		"GOFLAGS=-insecure go mod download",
		"npm config set registry https://registry.npmjs.org/",
		"pip config set global.index-url https://pypi.org/simple",
		"pip install git+https://github.com/x/y",
		"npm install https://example.com/pkg.tgz",
		"curl -sO https://files.pythonhosted.org/packages/x.whl",
		"wget https://registry.npmjs.org/left-pad/-/left-pad-1.3.0.tgz",
	}
	for _, c := range refused {
		if why := registryRefusal(p, c); !strings.Contains(why, "private-only") {
			t.Errorf("not refused: %s", c)
		}
	}
	allowed := []string{
		"npm install left-pad",
		"npm ci",
		"npm install x --registry https://art.example/npm/",
		"pip install -r requirements.txt",
		"pip install x -i https://art.example/pypi/simple/",
		"GOPROXY=https://art.example/go go mod download",
		"go build ./...",
		"go test ./...",
		"GOFLAGS=-mod=mod go build ./...",
		"echo npm install --registry https://registry.npmjs.org",
		"git push origin main",
	}
	for _, c := range allowed {
		if why := registryRefusal(p, c); why != "" {
			t.Errorf("refused %s: %s", c, why)
		}
	}
	// a manager with no private URL: its installs are refused, its env switched off
	bare := config.Packages{Policy: "private-only", Npm: "https://art.example/npm/"}
	for _, c := range []string{"pip install requests", "go get example.com/m", "uv add httpx"} {
		if why := registryRefusal(bare, c); !strings.Contains(why, "no private") {
			t.Errorf("no private URL, not refused: %s (%q)", c, why)
		}
	}
	env := strings.Join(bare.PackageEnv(), " ")
	if !strings.Contains(env, "GOPROXY=off") || !strings.Contains(env, "PIP_NO_INDEX=1") {
		t.Fatalf("private-only env: %s", env)
	}
	if env := strings.Join(p.PackageEnv(), " "); !strings.Contains(env, "GOPROXY=https://art.example/go") || strings.Contains(env, "direct") {
		t.Fatalf("private-only GOPROXY keeps no public fallback: %s", env)
	}
	// the all policy refuses nothing
	p.Policy = "all"
	for _, c := range refused {
		if why := registryRefusal(p, c); why != "" {
			t.Errorf("all refused %s", c)
		}
	}
}
