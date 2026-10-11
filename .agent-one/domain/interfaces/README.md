# domain-interfaces

- **Rank:** Domain owner
- **Owns:** `tui/`, `ui/`, `board/`, `theme/`
- **Reports to:** coord-agent-one
- **Skills:** domain-gate-review
- **Purpose:** rules what the operator sees — the terminal, the board and dashboard on loopback, the web ui system the agent builds pages with — and holds its gate.

## Invariants
- Neither the tui nor the board hard-codes agent-one's words: `tui.Words` and `board.Names` carry the labels and the templates read them; a new label is a new key in both.
- The board listens on 127.0.0.1 only; its acts (ask, answer, interrupt) need a POST whose Host is a loopback name and whose `X-Board-Token` is the page's own — the defence against DNS rebinding and cross-site requests (live.go).
- Golden renders live in `tui/testdata/golden/` (80 and 120 columns, TestBlocksGolden and TestShots); a rendering change regenerates them knowingly, never by accident. `ui check` needs a Chromium (UI_CHROMIUM, CHROME, or PATH); the ui lints also run at the review gate on any turn that touched the ui dir.
- Both faces wear one palette: `theme/` holds the named palettes (roles per dark and light); the terminal adapts it (tui/palette.go) and the dashboard's role sheet is generated from it (/theme.css). A palette passes the contrast table test in theme/ and the terminal's depth test, or is retuned.
- Zone workers: zone-terminal (tui/), zone-board (board/), zone-webui (ui/, theme/).

## Verify
- `go test ./tui/ ./ui/ ./board/`

## Working notes
