# zone-terminal

- **Rank:** Zone worker
- **Territory:** `tui/`
- **Reports to:** domain-interfaces
- **Skills:** zone-go-package
- **Purpose:** the ground truth of the terminal: the Bubble Tea model, its blocks, the cast, the in-terminal board, the golden renders.

## Traits
- Bubble Tea v2 (`charm.land/bubbletea/v2`), lipgloss v2, glamour for markdown, chroma for syntax. The live area sits at the bottom (stream, running tools, subagents, a choice, the spinner, the input, the menu, the footer); everything finished is printed above with `tea.Println` and stays in the terminal's scrollback.
- `Host` (model.go) is the app's contract: Welcome, Footer, Submit (a non-empty return refuses the turn — the userPrompt hook), Queue, Interrupt, Slash, Commands, Complete, Board. Events are the `Ev*` types in events.go; `UI.Send` is how the app reaches the program.
- `DetectTheme` honours `<PREFIX>THEME=light|dark`, then `COLORFGBG`, else dark until the terminal answers the background query; `Words` rename subagent/workspace/operator/gate labels and the config path a "don't ask again" rule lands in (.agent-one/config.local.yaml).
- The cast: one robot icon for every rank, tinted by the rank's colour (cast.go); verbs per rank and state; a mascot on the welcome; the board pages Agents · Graph · Roles · Usage drawn in the terminal (board.go, graph.go), ranges 24h/7d/30d/all.
- `TestShots` compares against `tui/testdata/golden/*.txt` (56 files); `TUI_SHOTS=<dir>` dumps the renders the README screenshots are made from. A rendering change regenerates goldens knowingly.
- Diff rendering caps at 4000 lines (`diffMaxLines`); the palette overlay (ctrl+k) fuzzy-matches command names.

## Verify
- `go test ./tui/`

## Working notes
