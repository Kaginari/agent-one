package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Kaginari/agent-one/config"
)

func regCfg(root string, p config.Packages) *config.Config {
	c := &config.Config{}
	c.Dist.WorkspaceDir = ".agent-one"
	c.Registry.Packages = p
	return c
}

// The package managers log in: npm through an npmrc whose bearer token stays a ${VAR} npm
// expands, pip and go through a netrc per host, both owner-only under the workspace's tmp; a
// per-manager login wins over the shared one; a login naming an unset variable is a hole and
// writes nothing for that manager; no login removes what an earlier start wrote.
func TestRegistryLogins(t *testing.T) {
	root := t.TempDir()
	env := map[string]string{"ART_TOKEN": "s3cret", "ART_USER": "ci", "GO_TOKEN": "g0"}
	get := func(k string) string { return env[k] }
	cfg := regCfg(root, config.Packages{
		Npm:     "https://art.example/api/npm/npm-remote/",
		Pip:     "https://art.example/api/pypi/pypi-remote/simple",
		Go:      "https://goproxy.example/go,direct",
		Auth:    config.PackageAuth{TokenEnv: "ART_TOKEN"},
		PipAuth: config.PackageAuth{UsernameEnv: "ART_USER", TokenEnv: "ART_TOKEN"},
		GoAuth:  config.PackageAuth{Username: "bot", TokenEnv: "GO_TOKEN"},
	})
	out, holes := registryAuth(cfg, root, get)
	if len(holes) != 0 {
		t.Fatalf("holes: %v", holes)
	}
	dir := registryAuthDir(cfg, root)
	npmrc, _ := os.ReadFile(filepath.Join(dir, "npmrc"))
	netrc, _ := os.ReadFile(filepath.Join(dir, "netrc"))
	if !strings.Contains(string(npmrc), "registry=https://art.example/api/npm/npm-remote/") ||
		!strings.Contains(string(npmrc), "//art.example/api/npm/npm-remote/:_authToken=${ART_TOKEN}") || strings.Contains(string(npmrc), "s3cret") {
		t.Fatalf("npmrc:\n%s", npmrc)
	}
	if strings.Contains(string(netrc), "art.example") || !strings.Contains(string(netrc), "machine goproxy.example login bot password g0") {
		t.Fatalf("netrc:\n%s", netrc)
	}
	for _, f := range []string{"npmrc", "netrc"} {
		if st, _ := os.Stat(filepath.Join(dir, f)); st.Mode().Perm() != 0o600 {
			t.Fatalf("%s mode %v", f, st.Mode().Perm())
		}
	}
	if j := strings.Join(out, " "); !strings.Contains(j, "NPM_CONFIG_USERCONFIG="+filepath.Join(dir, "npmrc")) || !strings.Contains(j, "NETRC="+filepath.Join(dir, "netrc")) ||
		!strings.Contains(j, "PIP_INDEX_URL=https://ci:s3cret@art.example/api/pypi/pypi-remote/simple") || !strings.Contains(j, "UV_INDEX_URL=https://ci:s3cret@art.example/") {
		t.Fatalf("env: %v", out)
	}
	if got := strings.Join(cfg.Registry.Packages.AuthEnv(), ","); got != "ART_TOKEN,ART_USER,GO_TOKEN" {
		t.Fatalf("the variables let through: %s", got)
	}

	// basic auth for npm: the token is base64'd into the file (npm cannot expand it so)
	cfg.Registry.Packages.NpmAuth = config.PackageAuth{Username: "ci", TokenEnv: "ART_TOKEN"}
	registryAuth(cfg, root, get)
	npmrc, _ = os.ReadFile(filepath.Join(dir, "npmrc"))
	if !strings.Contains(string(npmrc), ":username=ci") || !strings.Contains(string(npmrc), ":_password=czNjcmV0") {
		t.Fatalf("basic npmrc:\n%s", npmrc)
	}

	// an unset variable is a hole, and that manager gets no login
	delete(env, "GO_TOKEN")
	_, holes = registryAuth(cfg, root, get)
	netrc, _ = os.ReadFile(filepath.Join(dir, "netrc"))
	if len(holes) != 1 || !strings.Contains(holes[0], "$GO_TOKEN") || strings.Contains(string(netrc), "goproxy") {
		t.Fatalf("holes %v netrc:\n%s", holes, netrc)
	}

	// no login at all: nothing lingers
	cfg.Registry.Packages = config.Packages{Npm: "https://art.example/npm/"}
	if out, _ := registryAuth(cfg, root, get); out != nil {
		t.Fatalf("env without a login: %v", out)
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatal("the logins' dir survived")
	}
}
