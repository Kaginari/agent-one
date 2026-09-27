package ui

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// Widths are the catalogue's proving widths: a phone, a tablet, a desktop (README, Web UIs).
var Widths = []int{360, 768, 1280}

// Themes are rendered at every width.
var Themes = []string{"light", "dark"}

// Browser finds a headless-capable Chromium: $UI_CHROMIUM / $CHROME, then the usual names.
func Browser(env func(string) string) string {
	for _, k := range []string{"UI_CHROMIUM", "CHROME"} {
		if v := env(k); v != "" {
			return v
		}
	}
	for _, n := range []string{"chromium", "chromium-browser", "google-chrome", "google-chrome-stable", "chrome", "headless-shell"} {
		if p, err := exec.LookPath(n); err == nil {
			return p
		}
	}
	return ""
}

// Shot is one rendered view of the catalogue.
type Shot struct {
	Width    int    `json:"width"`
	Theme    string `json:"theme"`
	File     string `json:"file"`
	Overflow string `json:"overflow,omitempty"` // components that scroll sideways at this width
	Err      string `json:"err,omitempty"`
}

var reOverflow = regexp.MustCompile(`<html[^>]*\sdata-overflow="([^"]*)"`)

// Shoot renders the catalogue at every width in both themes into <workspace>/ui-assets/shots/.
// The pages are served from a loopback server over the ui dir and the legend only — a sandboxed
// browser (a snap) cannot read a hidden workspace dir, and nothing else under the root is served.
func Shoot(ctx context.Context, browser, root, workspaceDir string, m *Manifest, home string) ([]Shot, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}
	srv := &http.Server{Handler: serveOnly(root, []string{m.Dir, filepath.ToSlash(filepath.Join(workspaceDir, AssetsDir))}), ReadHeaderTimeout: 5 * time.Second}
	go srv.Serve(ln)
	defer srv.Close()
	base := fmt.Sprintf("http://%s/%s/%s/catalogue.html", ln.Addr(), filepath.ToSlash(workspaceDir), AssetsDir)
	out := filepath.Join(root, workspaceDir, AssetsDir, "shots")
	if err := os.MkdirAll(out, 0o755); err != nil {
		return nil, err
	}
	stage, cleanup, err := stagingDir(browser, home)
	if err != nil {
		return nil, err
	}
	defer cleanup()
	var shots []Shot
	for _, w := range Widths {
		for _, th := range Themes {
			name := fmt.Sprintf("catalogue-%d-%s.png", w, th)
			url := base + "#theme=" + th
			s := Shot{Width: w, Theme: th, File: filepath.ToSlash(filepath.Join(workspaceDir, AssetsDir, "shots", name))}
			size := fmt.Sprintf("--window-size=%d,%d", w, 1000)
			tmp := filepath.Join(stage, name)
			if msg, err := chrome(ctx, browser, size, "--screenshot="+tmp, url); err != nil {
				s.Err = strings.TrimSpace(err.Error() + " " + msg)
			} else if err := move(tmp, filepath.Join(out, name)); err != nil {
				s.Err = err.Error()
			}
			if dom, err := chrome(ctx, browser, size, "--dump-dom", url); err == nil {
				if mm := reOverflow.FindStringSubmatch(dom); mm != nil {
					s.Overflow = mm[1]
				}
			}
			shots = append(shots, s)
		}
	}
	return shots, nil
}

func chrome(ctx context.Context, browser string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	full := append([]string{"--headless", "--disable-gpu", "--hide-scrollbars", "--no-first-run", "--no-default-browser-check", "--virtual-time-budget=3000"}, args...)
	if os.Getuid() == 0 {
		full = append([]string{"--no-sandbox"}, full...) // a root container has no user namespace
	}
	cmd := exec.CommandContext(ctx, browser, full...)
	var stdout, stderr strings.Builder
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	err := cmd.Run()
	if err != nil {
		return lastLine(stderr.String()), err
	}
	return stdout.String(), nil
}

// stagingDir is where the browser may write: a snap only writes under ~/snap/<name>/common.
func stagingDir(browser, home string) (string, func(), error) {
	real, _ := filepath.EvalSymlinks(browser)
	snap := strings.HasPrefix(browser, "/snap/") || strings.HasPrefix(real, "/snap/") || isSnapWrapper(browser)
	if snap && home != "" {
		d := filepath.Join(home, "snap", "chromium", "common", fmt.Sprintf("ui-shots-%d", os.Getpid()))
		if err := os.MkdirAll(d, 0o755); err == nil {
			return d, func() { os.RemoveAll(d) }, nil
		}
	}
	d, err := os.MkdirTemp("", "ui-shots-")
	return d, func() { os.RemoveAll(d) }, err
}

// isSnapWrapper spots Ubuntu's /usr/bin/chromium-browser, a script that execs the snap.
func isSnapWrapper(p string) bool {
	b, err := os.ReadFile(p)
	if err != nil || len(b) > 64<<10 {
		return false
	}
	return strings.Contains(string(b), "/snap/bin/chromium")
}

func move(from, to string) error {
	if err := os.Rename(from, to); err == nil {
		return nil
	}
	b, err := os.ReadFile(from)
	if err != nil {
		return fmt.Errorf("the browser wrote no screenshot: %w", err)
	}
	return os.WriteFile(to, b, 0o644)
}

// serveOnly serves files under the named dirs of root, and nothing else.
func serveOnly(root string, dirs []string) http.Handler {
	fs := http.FileServer(http.Dir(root))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := strings.TrimPrefix(filepath.ToSlash(filepath.Clean("/"+r.URL.Path)), "/")
		for _, d := range dirs {
			if p == d || strings.HasPrefix(p, strings.TrimSuffix(d, "/")+"/") {
				fs.ServeHTTP(w, r)
				return
			}
		}
		http.NotFound(w, r)
	})
}

func lastLine(s string) string {
	lines := strings.Split(strings.TrimSpace(s), "\n")
	return lines[len(lines)-1]
}
