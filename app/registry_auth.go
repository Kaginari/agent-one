package app

import (
	"encoding/base64"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/Kaginari/agent-one/config"
)

// The package registries' logins (registry.packages.auth, npmAuth, pipAuth, goAuth). The
// managers do not read one shared variable, so the harness writes what each one reads, under
// the workspace's tmp (gitignored), owner-only, rewritten at every start:
//   - npm: an npmrc (NPM_CONFIG_USERCONFIG) — the registry and its auth line; a token named by
//     tokenEnv stays a ${VAR} reference npm expands itself, so it is never written;
//   - pip and uv: the login rides in PIP_INDEX_URL / UV_INDEX_URL (https://user:token@host/…),
//     which every pip reads (an older pip ignores NETRC);
//   - go: a netrc (NETRC) — `machine <host> login <user> password <token>`, one line per host
//     GOPROXY names; go sends it over https only. netrc has no variables: the token is written there.
// No login configured: the directory is removed, so a token from an earlier start never lingers.

// registryAuthDir is where the logins are written.
func registryAuthDir(cfg *config.Config, root string) string {
	return filepath.Join(root, cfg.Dist.WorkspaceDir, "tmp", "registry")
}

// registryAuth writes the logins and returns the environment that points the managers at them,
// and a hole per login that names a variable not set.
func registryAuth(cfg *config.Config, root string, getenv func(string) string) (env, holes []string) {
	p := cfg.Registry.Packages
	dir := registryAuthDir(cfg, root)
	resolve := func(manager string) (user, token string, ref string, ok bool) {
		a := p.AuthFor(manager)
		if !a.Set() {
			return "", "", "", false
		}
		user = a.Username
		if a.UsernameEnv != "" {
			if v := getenv(a.UsernameEnv); v != "" {
				user = v
			} else if user == "" {
				holes = append(holes, fmt.Sprintf("registry: %s's login names $%s, which is not set", manager, a.UsernameEnv))
			}
		}
		token = a.Token
		if a.TokenEnv != "" {
			ref = a.TokenEnv
			if v := getenv(a.TokenEnv); v != "" {
				token = v
			} else if token == "" {
				holes = append(holes, fmt.Sprintf("registry: %s's login names $%s, which is not set", manager, a.TokenEnv))
				return "", "", "", false
			}
		}
		return user, token, ref, token != ""
	}

	var npmrc []string
	if u, err := url.Parse(p.Npm); p.Npm != "" && err == nil && u.Host != "" {
		if user, token, ref, ok := resolve("npm"); ok {
			scope := "//" + u.Host + strings.TrimSuffix(u.Path, "/") + "/"
			npmrc = append(npmrc, "registry="+p.Npm)
			switch {
			case user != "": // basic: npm wants the password base64'd, so the token is written
				npmrc = append(npmrc, scope+":username="+user, scope+":_password="+base64.StdEncoding.EncodeToString([]byte(token)), scope+":always-auth=true")
			case ref != "" && getenv(ref) != "": // bearer by reference: npm expands it, nothing written
				npmrc = append(npmrc, scope+":_authToken=${"+ref+"}", scope+":always-auth=true")
			default:
				npmrc = append(npmrc, scope+":_authToken="+token, scope+":always-auth=true")
			}
		}
	}

	machines := map[string]string{}
	add := func(manager string, raw []string) {
		user, token, _, ok := resolve(manager)
		if !ok {
			return
		}
		if user == "" {
			user = "token"
		}
		for _, r := range raw {
			r = strings.TrimSpace(r)
			if u, err := url.Parse(r); err == nil && u.Host != "" {
				machines[u.Host] = "machine " + u.Host + " login " + user + " password " + token
			}
		}
	}
	if u, err := url.Parse(p.Pip); p.Pip != "" && err == nil && u.Host != "" {
		if user, token, _, ok := resolve("pip"); ok {
			if user == "" {
				user = "token"
			}
			u.User = url.UserPassword(user, token)
			env = append(env, "PIP_INDEX_URL="+u.String(), "UV_INDEX_URL="+u.String())
		}
	}
	if p.Go != "" {
		add("go", strings.FieldsFunc(p.Go, func(r rune) bool { return r == ',' || r == '|' }))
	}

	if len(npmrc) == 0 && len(machines) == 0 {
		_ = os.RemoveAll(dir)
		return env, holes
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, append(holes, "registry: the logins' dir: "+err.Error())
	}
	_ = os.Chmod(dir, 0o700)
	write := func(name string, lines []string) (string, bool) {
		path := filepath.Join(dir, name)
		if len(lines) == 0 {
			_ = os.Remove(path)
			return "", false
		}
		if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0o600); err != nil {
			holes = append(holes, "registry: "+name+": "+err.Error())
			return "", false
		}
		_ = os.Chmod(path, 0o600)
		return path, true
	}
	if path, ok := write("npmrc", npmrc); ok {
		env = append(env, "NPM_CONFIG_USERCONFIG="+path)
	}
	var lines []string
	for _, l := range machines {
		lines = append(lines, l)
	}
	sort.Strings(lines)
	if path, ok := write("netrc", lines); ok {
		env = append(env, "NETRC="+path)
	}
	return env, holes
}
