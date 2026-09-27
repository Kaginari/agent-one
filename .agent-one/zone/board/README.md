# zone-board

- **Rank:** Zone worker
- **Territory:** `board/`
- **Reports to:** domain-interfaces
- **Skills:** zone-go-package, zone-workspace-docs
- **Purpose:** the ground truth of the board and the dashboard: the http.Handler on loopback that reads the same instruments the session writes.

## Traits
- Everything served is embedded: Bootstrap 5 in `board/assets` (with SHA256SUMS and LICENSE — the one directory the leak test skips as vendored), `board/static`, `board/templates`, and `board/web` for the dashboard (its own tokens.css, palettes.css, layout.css, components and portraits). A page works offline.
- Pages: `/` overview, `/subagent`, `/usage`, `/team`, `/memory`, `/toolbox`, `/log`, `/config`; JSON at /api/doc, `/api/subagent`, `/api/usage`, `/api/team`; `/events` is SSE fed by a file watcher (`Poll` default 1 s). The dashboard's acts (ask, answer, interrupt) live under `liveRoutes` and pass `local(r)` (Host is localhost, 127.0.0.1 or ::1 and Origin matches) plus the `X-Board-Token` header; a POST that fails either is 403.
- Sources read instruments/usage/*.jsonl, instruments/loop/, `log.md` (`ParseLog`: `### [ts] author — title`), and the ontology through `BuildTeam` (lanes zone · review · global · shared, decided by the holders' ranks). Every reading carries its source and age, or why it is silent.
- `Names` (DefaultNames) carry the lexicon labels; no agent-one term is hard-coded in a template. /api/doc serves a member doc only from inside the workspace (`errOutside`).
- `Prefix` lets the board mount under a path; `Live` nil makes it read-only (`agent-one board`).

## Verify
- `go test ./board/`

## Working notes
