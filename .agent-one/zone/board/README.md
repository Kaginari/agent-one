# zone-board

- **Rank:** Zone worker
- **Owns:** `board/`
- **Reports to:** domain-interfaces
- **Skills:** zone-go-package, zone-workspace-docs
- **Purpose:** the ground truth of the board and the dashboard: the http.Handler on loopback that reads the same instruments the session writes.

## Invariants
- `board.New(Options)` is an http.Handler with everything embedded (web/: the one page, its components, the realm theme and vendored OFL fonts — no CDN, no Bootstrap): it works offline. The app binds it to 127.0.0.1 on ui.board.port (default 7411); `agent-one board` serves it read-only without a session. `board/README.md` documents the feeds and the usage schema (a test holds the schema doc true).
- Routes: `/` is the one page — the dashboard, its tabs (Overview, Console, Neural net, Metrics, Subagent, Memory, Toolbox, Relations, Tree, Config) deep-linked by `/#tab`; `/dash` and the old page paths (subagent, memory, toolbox, config, usage, team) answer 301 to their tab; `/log`, `/static/`, `/assets/` are gone (404). /events (SSE; watch polls file signatures — name, size, mtime); /api/doc subagent usage team activity tree tree/children subagents plan, and workspace.go's /api/runs run memory memory/recall memory/review toolbox config — every read is `guarded` (a loopback Host only: a rebound DNS name reads nothing). liveRoutes add /web/, /dash/stream, /api/workspace, /api/live and the acts /api/ask, /api/answer, /api/interrupt; themeRoutes add /theme.css (the palette's roles, generated — Options.Theme, ?name= picks one) and /api/theme (GET the palettes and the look; POST from the page's own token and a loopback Host, no session needed, through Options.SetTheme, 409 without one). web/theme.css holds no role value — only the faces, the meaning tier, the glows derived from the roles; the page arrives in the look ({{SCHEME}}, {{MODE}}) and swaps the role sheet in place on a pick or a `theme` event, setting data-scheme for the 3D net. /api/subagents sends counts only; /api/config blanks credential values; /api/tree/children serves one lazy level and 404s a path escaping the root.
- Acts pass `act()` or nothing happens: POST only (405), local() — Host is localhost, 127.0.0.1 or ::1 and Origin, when present, is this host (DNS rebinding) — and X-Board-Token equal to this board's random per-run token (403), and a Live attached (409 otherwise). /dash/stream also refuses a non-local Host.
- The answer path: Live (board/live.go) is the interface the app's webBus implements — Subscribe (replay, then the stream), Ask, Answer(id, index, text), Interrupt, Status. The board never holds a session itself; /dash/stream sends `reset`, the replay, then events with a 15 s ping, or `nosession` when Live is nil.
- `/` serves web/dash.html with {{TOKEN}}, {{WORDS}}, {{DIST}} substituted, Cache-Control no-store and a strict CSP (script-src 'self', no inline script, frame-ancestors 'none'). dash.js has no dependencies, builds DOM with textContent or escaped markup and sends the token on every act; the net is components/net (net.js 2D lanes, net3d.js a WebGL2 stage) over /api/team and /api/activity.
- The dash is built on the ui system: web/tokens.css, palettes.css and layout.css are byte copies of the ui foundation, and web/components/<name>/ must lint clean under ui.Lint — TestDashboardHoldsTheUISystem fails on drift ("copy it again"). Colours and lengths come from tokens only.
- Names maps every word a page shows through the lexicon (DefaultNames); a new page word goes into Names, never literally into a template or dash.js.
- `board/sources.go` reads the instruments: every figure carries a Reading (lit or silent, why); a silent or missing instrument renders as "silent since …" or "never lit", never as zero. Usage rollups treat a null usd as unpriced.
- The team reads onto.Cached, so a poll with nothing moved on disk costs a stat walk, not a load.

## Verify
- `go test ./board/`

## Working notes
