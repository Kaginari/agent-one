# zone-terminal

- **Rank:** Zone worker
- **Owns:** `tui/`
- **Reports to:** domain-interfaces
- **Skills:** zone-go-package
- **Purpose:** the ground truth of the terminal: the Bubble Tea model, its blocks, the cast, the in-terminal board, the golden renders.

## Invariants
- Layers: `tui/blocks.go` is the view model (pure Theme methods from views to styled text), `tui/model.go` the Bubble Tea Model over it, `tui/program.go` Start/Send/Quit. Host (model.go) is what the app implements: Welcome, Footer, Submit (a non-empty string refuses the turn), Queue, Interrupt, Slash, Pipe, Commands, Complete, Board, Plans. `tui/events.go` is the message set the app sends (EvDelta, EvToolStart/End, EvSubagent, EvState, EvTurnStart/Done, EvNotice, EvError, EvLines, EvChoice/ChoiceAnswer, EvChoiceAnswered, EvPlanOutcome, EvQuit).
- Pane mode: railSize gives the rail a third of the window (floor railFloor 28) once the window is at least 60 wide and railMinH 26 tall; below that the session is one column printed into the terminal's scrollback with tea.Println. With the rail, the frame is rectangle-exact (always m.height rows): the chat pane is a viewport (`tui/pane_vp.go` — pgup/pgdn, shift+↑/↓, the wheel; it follows the bottom until the human scrolls up — follow is read before a resize, so a shrinking pane never reads as a climb; slash output and every other block land in it, never a tea.Println above the frame) beside the rail's cards (vitals, plan, subagents, files) that scroll with the wheel over the rail and fold with a click on their header or alt+p / alt+c / alt+f.
- Render cache: blocks are append-only; `rendered` caches each block's text per width and renderGen (bumped on a theme swap), and paneChat extends its line cache per landed block, rebuilding only on a width, reflow or theme change; the viewport is fed only when paneVer or the live rows moved. An idle frame must not re-render.
- Plans load off the UI goroutine: the tick issues one `host.Plans()` tea.Cmd at a time (plansBusy) and evPlans lands it. ctrl+p cycles decks (sticky); alt+n snaps back to the session's own live, non-archived, non-stale orchestrator deck. Rail chords are alt/ctrl chords, never plain runes — a letter would eat a typed word.
- Selection and copy (`tui/select.go`): while the panes own the window the program holds the mouse, so a drag in the chat pane selects in content coordinates (double/triple click word/line, autoscroll at the edges) and the release copies plain text through OSC 52, the primary selection and a native tool (pbcopy, wl-copy, xclip, xsel; never over SSH, off the UI goroutine). ctrl+y and /copy copy the last answer's markdown; /mouse hands the mouse back to the terminal (footer badge).
- Keys: esc clears a selection, a menu, interrupts a turn or clears the draft (↑ brings it back); ctrl+c clears a draft, then interrupts, then needs a second press within 2 s to exit; ctrl+o expands the last folded block; ctrl+k the palette; ctrl+t the agents view; /board is full-screen (pages Agents, Graph, Roles, Usage; BoardRanges 24h 7d 30d all). The footer draws `ctx` as a meter at width ≥ 72 and adds raw `tokens/limit` at ≥ 88.
- Theme: the palette is package theme's (`tui/palette.go` adapts it; NewThemeFrom is the one constructor that reads roles); `tui/look.go` holds the look — DetectThemeFor(env, prefix, ui.theme, ui.mode) at start (<PREFIX>_THEME wins, system asks the terminal), /theme and /settings switch live and hand the switch to a ThemeHost, EvTheme follows one made elsewhere; a switch keeps the terminal's Color/TrueColor. NoColor gives a plain theme; markdown through glamour, code through chroma, diffs capped by `tui/diff.go`; cast.go draws one robot icon tinted by the rank's colour, with verbs per rank and state.
- Value meters (`tui/rail.go`): FooterView carries Meters (cfg.Meters), Ranks, Values, Score and Promote; gauges band by meterDef/meterHue (never numbers of their own); the ranks tile shows value meters when any rank has a record, else each rank's token share; rankHue falls back to the family's lane for a custom rank.
- Goldens live in `tui/testdata/golden` at 80 and 120 columns with escapes stripped; `go test ./tui/ -run TestBlocksGolden -args -goldens` rewrites them and the diff is looked at, not only accepted. TestShots writes ANSI frames only when TUI_SHOTS names a directory. A visual change refreshes the PNGs in `docs/screens/`.

## Verify
- `go test ./tui/`

## Working notes
