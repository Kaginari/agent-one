package board

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
)

// TestDashboardFacesAreVendoredAndServed: every face theme.css declares is embedded and served
// from the board itself (the page's CSP is self-only) as font/woff2, each family's OFL beside it,
// and the page picks the realm theme, dark until the reader chooses.
func TestDashboardFacesAreVendoredAndServed(t *testing.T) {
	css, err := content.ReadFile("web/theme.css")
	if err != nil {
		t.Fatal(err)
	}
	urls := regexp.MustCompile(`url\("(fonts/[^"]+\.woff2)"\)`).FindAllStringSubmatch(string(css), -1)
	if len(urls) < 3 {
		t.Fatalf("theme.css declares %d faces, want the display, agent and code faces", len(urls))
	}
	b := newBoard(t, "workspace", Options{})
	for _, u := range urls {
		rec := serve(t, b, "/web/"+u[1])
		if rec.Code != 200 || rec.Header().Get("Content-Type") != "font/woff2" || rec.Body.Len() < 1000 {
			t.Errorf("%s: %d %q, %d bytes", u[1], rec.Code, rec.Header().Get("Content-Type"), rec.Body.Len())
		}
	}
	for _, lic := range []string{"OFL-Orbitron.txt", "OFL-Rajdhani.txt", "OFL-JetBrainsMono.txt"} {
		if src, err := content.ReadFile("web/fonts/" + lic); err != nil || !strings.Contains(string(src), "SIL Open Font License") {
			t.Errorf("web/fonts/%s: the face ships without its licence (%v)", lic, err)
		}
	}
	_, page := get(t, b, "/")
	if !strings.Contains(page, `data-palette="realm" data-scheme="cyberpunk" data-theme="dark"`) || !strings.Contains(page, `href="web/theme.css"`) ||
		!strings.Contains(page, `href="theme.css?name=cyberpunk"`) {
		t.Error("the page loads the palette's roles and theme.css, and starts in the default look: cyberpunk, dark")
	}
}

// /theme.css is the palette's roles, generated: the look in force, or the one named; every role
// theme.css's meaning tier reads is set in it, and theme.css itself holds no role value.
func TestThemeStylesheet(t *testing.T) {
	b := newBoard(t, "workspace", Options{Theme: func() (string, string) { return "synthwave", "system" }})
	rec := serve(t, b, "/theme.css")
	css := rec.Body.String()
	if rec.Code != 200 || !strings.HasPrefix(rec.Header().Get("Content-Type"), "text/css") || !strings.Contains(css, "theme synthwave") || !strings.Contains(css, "--realm-primary: #ff4dff;") {
		t.Fatalf("/theme.css: %d %q\n%s", rec.Code, rec.Header().Get("Content-Type"), css)
	}
	if css := serve(t, b, "/theme.css?name=glitch").Body.String(); !strings.Contains(css, "theme glitch") {
		t.Fatal("?name= picks a palette")
	}
	if css := serve(t, b, "/theme.css?name=nope").Body.String(); !strings.Contains(css, "theme synthwave") {
		t.Fatal("an unknown name is the look in force")
	}
	static, _ := content.ReadFile("web/theme.css")
	for _, m := range regexp.MustCompile(`var\((--realm-[\w-]+)\)`).FindAllStringSubmatch(string(static), -1) {
		if !strings.Contains(css, m[1]+":") {
			t.Errorf("theme.css reads %s, which /theme.css does not set", m[1])
		}
	}
	if regexp.MustCompile(`--realm-[\w-]+:`).Match(static) {
		t.Error("theme.css sets a role: the roles are the palette's, generated")
	}
	_, page := get(t, b, "/")
	if !strings.Contains(page, `data-scheme="synthwave">`) {
		t.Error("system mode leaves data-theme to the browser")
	}
}

// /api/theme lists every palette and the look in force; a POST switches it — from this board's
// own page only (loopback, the token), an unknown palette refused with the list, the integrator's
// SetTheme called with the full look (a mode alone keeps the palette).
func TestThemeAPI(t *testing.T) {
	look := [2]string{"cyberpunk", "dark"}
	var set [][2]string
	b := newBoard(t, "workspace", Options{Theme: func() (string, string) { return look[0], look[1] },
		SetTheme: func(n, m string) error { set = append(set, [2]string{n, m}); look = [2]string{n, m}; return nil }})
	code, agent := get(t, b, "/api/theme")
	var got struct {
		Name, Mode string
		Settable   bool
		Themes     []struct{ Name string }
	}
	if err := json.Unmarshal([]byte(agent), &got); code != 200 || err != nil || got.Name != "cyberpunk" || !got.Settable || len(got.Themes) != 8 {
		t.Fatalf("/api/theme: %d %v %s", code, err, agent)
	}
	post := func(agent, token string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest("POST", "/api/theme", strings.NewReader(agent))
		req.Host = "127.0.0.1:7482"
		req.Header.Set("X-Board-Token", token)
		b.ServeHTTP(rec, req)
		return rec
	}
	if rec := post(`{"name":"rain-city"}`, "wrong"); rec.Code != 403 || len(set) != 0 {
		t.Fatalf("a page without the token switched the look: %d", rec.Code)
	}
	if rec := post(`{"name":"vaporwave"}`, b.Token()); rec.Code != 400 || !strings.Contains(rec.Body.String(), "rain-city") {
		t.Fatalf("an unknown palette: %d %s", rec.Code, rec.Body.String())
	}
	if rec := post(`{"name":"Rain-City"}`, b.Token()); rec.Code != 200 || set[0] != [2]string{"rain-city", "dark"} {
		t.Fatalf("a switch: %d %v", rec.Code, set)
	}
	if rec := post(`{"mode":"light"}`, b.Token()); rec.Code != 200 || set[1] != [2]string{"rain-city", "light"} {
		t.Fatalf("a mode alone keeps the palette: %d %v", rec.Code, set)
	}
	ro := newBoard(t, "workspace", Options{})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/api/theme", strings.NewReader(`{"name":"glitch"}`))
	req.Host = "127.0.0.1:7482"
	req.Header.Set("X-Board-Token", ro.Token())
	ro.ServeHTTP(rec, req)
	if rec.Code != 409 {
		t.Fatalf("a board that cannot keep a look says so: %d", rec.Code)
	}
}

func serve(t *testing.T, h http.Handler, path string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", path, nil))
	return rec
}
