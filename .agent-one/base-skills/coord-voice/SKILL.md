---
name: coord-voice
description: The voice of agent-one across domains — its words, its promises, and the project-wide files that carry them (README, CHANGELOG, the policy in two copies, go.mod). Wear it when speaking for the workspace, routing a cross-domain change, or editing a project-wide file.
---
- Words: operator (the human), orchestrator (the session agent), coordinator · domain owner · zone worker · service owner (`coord- domain- zone- service-`), subagent, workspace, the wire (`@S @F @V @? @U @E`), the review gate, the human gate, the guard, the sandbox, the drain, the shelf, the toolbox, the board. Analyst reads, judge verdicts, drafter drafts.
- Never the words of the convention this engine was forked from: the leak test names them and fails the module on any file, docs included. When in doubt, `go test .`.
- A README claim is a promise enforced in code: find the package before writing it (review gate → `workspace/gate.go`; approval → `gate/`; sandbox → `sandbox/`, `shell/`; drain → `compact/`; config layers → `config/load.go`). "Everything on, configuration takes away; a disabled policy is reported, never silent" is the standing promise (`config.Off`, `status`).
- The policy lives twice: `AGENT-ONE.md` at the root and `app/policy/AGENT-ONE.md` embedded; a test keeps them equal. It changes only on the operator's order (Policy 6), both copies in one change.
- Cross-domain routing: a change that touches two domains goes to both owners; owners may talk sideways (domain ⇄ domain); anything a zone cannot own goes up one hop, never across.
- Project-wide files: `README.md`, `CHANGELOG.md` (Keep-a-Changelog sections; the release fails without the tag's section), `go.mod`/`go.sum` (`go 1.26.8`; `go mod tidy` is a GoReleaser pre-hook), `LICENSE` (MIT; Tabler Icons and Bootstrap credited), `docs/screens/*.png` made from `TUI_SHOTS` dumps, `examples/gateway/README.md` (the corp-gateway configuration the README quotes).
- Reports go up on the wire, terse: findings as `@F`, verdicts as `@V`, holes as `@?`, the unsaid as `@U kind: text`.

## Working notes
