package board

import (
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
	page, _ := content.ReadFile("web/dash.html")
	if !strings.Contains(string(page), `data-palette="realm" data-theme="dark"`) || !strings.Contains(string(page), `href="web/theme.css"`) {
		t.Error("dash.html must load theme.css and start in the realm theme, dark")
	}
}

func serve(t *testing.T, h http.Handler, path string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", path, nil))
	return rec
}
