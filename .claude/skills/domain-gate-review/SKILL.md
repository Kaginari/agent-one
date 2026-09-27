---
name: domain-gate-review
description: The domain owner's review checklist for a landing in this repository — the four policy checks made concrete, plus the repo's own gates (gofmt, vet, leak test, policy copy, goldens, tests intact). Wear it when passing or failing a zone worker's change.
---
1. Right author: every written path is inside the authoring zone's `Territory` (prefix match, file lists for `app/` and `config/`); a write elsewhere is a fail with the escalation hint, not a fix by you.
2. Invariants hold: run the zone's verify lines as they stood before the change (`go test ./<pkg>/`, the `-run` subsets for `app/` zones). A verify line the change edited is a hole you confirm explicitly.
3. Duties done: the ask's every part landed; a wire answer carries `@S`; `+unsaid` asked means `@U` came back.
4. Doc truthful: the zone's README changed in the same change when the territory's behaviour did (Docs-as-code) — and the package's `// Package` comment, the README section that promises it, and `CHANGELOG.md` `## [Unreleased]` when the operator will notice.
- Repo gates on top: `test -z "$(gofmt -l .)"`, `go vet ./...`, `go test .` (the leak test — over `.agent-one/` and `.claude/` too), `go test ./app/ -run TestEmbeddedLawsMatchTheWorkspace` when `AGENT-ONE.md` moved.
- Tests intact: a turn does not pass by deleting a test, dropping a `func Test…`, or adding `t.Skip` — the gate's `testsIntact` reads the same; only the operator retires a test.
- Goldens: a change under `tui/` that regenerates `tui/testdata/golden/*.txt` must say so and why; a golden that changed with no rendering intent is a fail.
- Safety domain extra: a class or pattern change ships with a corpus line; nothing in the ladder got looser; the scrub list only grew.
- Record the verdict in `.agent-one/log.md` (`Gate: pass|fail — reason`) — append, never edit.

## Working notes
