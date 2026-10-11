# board — the workspace, seen

`board.New(Options{WorkspaceRoot, WorkspaceDir, Names, Sources})` is an `http.Handler`; `board.Serve(ctx,
addr, opts)` listens until the context ends. It serves one page — the dashboard, `web/dash.html`,
at `/` — and everything it needs is embedded under `web/` (the ui system's tokens, palettes,
layout and components, the realm theme, the vendored faces), so the page works offline.

`Board.Token()` returns the per-session token that authorizes dashboard acts
(`/api/ask`, `/api/answer`, `/api/interrupt`). The token is registered in `New` and embedded in the
page (`{{TOKEN}}`); `dash.js` sends it as `X-Board-Token`. Exposed for tests that need
to act as the page (e.g., `TestDashGateAnswerOverHTTP`).

Tabs, one question each: Overview (the cockpit) · Console · Neural net · Metrics · Subagent · Memory ·
Toolbox · Relations · Tree · Config. `dash.js` draws the cockpit and the session's views;
`workspace.js` draws the workspace's tabs (Subagent, Memory, Toolbox, Config) from the JSON below. A deep
link opens a tab (`/#memory`, `/#subagent/<run id>`). Labels come from `Options.Names` (keys in
`DefaultNames`); a distribution passes its own words.

Old addresses: `/dash` → `/` (301); `/subagent`, `/memory`, `/toolbox`, `/config`, `/usage`,
`/team` → their tab (301). Anything else that is not an API, `/events`, `/dash/stream` or
`/web/…` is a 404.

## The workspace's JSON

Read-only, and answered only to a loopback Host (a rebound DNS name cannot read them):

| path | answers |
|---|---|
| `/api/runs[?as=agent]` | every journaled run, newest first: id, as, ask (words, no wire header), status, at, turns, steps, failed, provider; and the reading |
| `/api/run?id=` | one run whole: the commission verbatim, each step (tool, gate class, decision and why, summary, status, attempts, ms), the end's holes and length, the context reading; 404 for an unknown id |
| `/api/memory` | the tiers (`status.short/long/shared`), the working notes, the shared notes newest first, notes by kind, the holes |
| `/api/memory/recall?q=&tier=&kind=` | `memory.Recall` with `NoCache` — the twelve best memories, never a cache line written |
| `/api/memory/review` | `memory.Review` — each tier's facts and routed findings |
| `/api/toolbox` | the registry's entries with their state (loaded · offered · idle), offers and loads; the instrument; the loads journal's newest 40 lines |
| `/api/config` | the integrator's config tree (`{value, origin}` leaves), credentials redacted, and the off-list |

## Feeds

`Sources` is a struct of functions; a nil field falls back to the file-backed reader
(`FileSources`). The integrator wires what files cannot know:

| field | answers | default |
|---|---|---|
| `Subagent() []Agent` | the live agents (rank, role, model, state, started, context, usage, usd) | silent |
| `Config() any` | the effective config; rendered generically as a tree, a `{value, origin}` map is a leaf with its origin badge | silent |
| `Off() []Off` | policy features disabled by config, with the origin | silent |
| `Session() Session` | the running session | the last session in the usage journal |
| `Usage`, `Loop`, `Log`, `Team`, `Memory`, `Toolbox`, `Doc` | files: `instruments/usage/*.jsonl`, `instruments/loop/*.jsonl`, `log.md`, packages `onto`, `memory`, `toolbox` | file-backed |

Every figure carries a `Reading` (source, last time, lit or silent, why). A silent or missing
instrument renders as "silent since …" or "never lit", never as zero.

## The usage journal

Pairing line (2026-10-06): `/api/plan` reads the query key `agent` (not `deck`); FileSources populates `Plan` from `<workspaceDir>/tmp/plans/<body>.json` — a missing file or an empty `items` list returns `{"agent": name, "empty": true}` with 200, never an error status. dash.js fetches `/api/plan?agent=<id>` inside the net knowledge card to render the plan headlines.

`<workspaceDir>/instruments/usage/<session>.jsonl`, one JSON object per provider call:

```json
{"ts":"2026-09-26T08:00:00Z","session":"s-2026-09-26","agent":"zone-auth","rank":"zone",
 "role":"findings","model":"claude-opus-5","provider":"anthropic",
 "input":800,"output":900,"cacheRead":0,"cacheWrite":0,"usd":0.0159}
```

| field | type | meaning |
|---|---|---|
| `ts` | RFC 3339 | when the call returned |
| `session` | string | session id (defaults to the file name) |
| `agent` | string | the agent that made the call (`orchestrator`, `zone-auth`, …) |
| `rank` | string | its rank (`orchestrator`, `coord`, `domain`, `zone`, `service`, or a config rank) |
| `role` | string | `findings` · `verdict` · `draft` · `""` |
| `model`, `provider` | string | as resolved for the call |
| `input`, `output`, `cacheRead`, `cacheWrite` | int | tokens as the provider reported them |
| `usd` | number or `null` | priced from config; `null` = unpriced (shown as tokens, never a guessed cost) |

Unreadable lines are skipped and counted in the reading. Rollups (by agent, role, rank, model,
day) treat `null` as unpriced: the USD column is a floor and says how many calls it misses.

## Live

`/events` is server-sent events: `usage`, `loop`, `log`, `memory`, `toolbox` (a file changed
under the workspace dir) and `subagent` (the live feed's JSON changed; the payload carries the agents).
The page refetches the JSON a tab reads when its instrument moves.

## Theme

`web/theme.css` is the realm theme over the foundation (`tokens.css`, `palettes.css`,
`layout.css`); components read only the meaning tier, each declaring what it reads in its header
(`ui check` holds them to it — `TestDashboardHoldsTheUISystem`).
