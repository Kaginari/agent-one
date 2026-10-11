# Chronicle — change log

Append-only. Newest entries at the bottom. One entry per change.

---

### [2026-09-27T23:14:00+02:00] orchestrator — founded the team of agent-one (init --level complex)
- **Task:** read every package, the build, CI and release files; decide and write the team, its skills and agents; verify every verify line; pass onto check and the leak test
- **Files:** .agent-one/coord/agent-one/, .agent-one/domain/{engine,safety,workspace,interfaces,config,app}/, .agent-one/zone/{loop,providers,drain,tools,shell,guard,mcp,policy,ontology,memory,discovery,terminal,board,webui,config,yaml,cli,session,shelf}/, .claude/skills/{zone-go-package,zone-act-classes,zone-workspace-docs,domain-gate-review,coord-voice,service-release-flow}/, .claude/agents/{coord-agent-one,domain-engine,domain-safety,domain-workspace,domain-interfaces,domain-config,domain-app,service-release}.md
- **Gate:** n/a — founding; policy.gate.rightAuthor is off for this session by init
- **Result:** 1 coordinator, 6 domain owners, 17 zone workers, 1 service owner, 6 skills, 8 agents; every verify line ran and passed before it was written; build, vet, gofmt and the whole test suite pass today
- **Learned:**
  - bench/fakevllm is its own Go module (module fakevllm): go build, vet and test ./... from the root never reach it; build it with go build -C bench/fakevllm
  - config.Dist.RankDirs is "coord,domain,zone" — service is missing from the [rank-dirs] default, while the lexicon and the ontology both know service
  - bench/harbor/__pycache__ is root-owned (Harbor's Docker runner), so python3 -m py_compile fails on this tree; syntax is checked with ast.parse instead
  - the ontology counts every back-ticked path with a slash anywhere in a member doc as ownership: traits that name files outside a zone's territory must not back-tick them
  - ownership at runtime is prefix-based with no globs: zones that share app/ or config/ list their files one by one
  - the README's CLI table and app/cli.go's commands table are tied by no test; the two policy copies are (TestEmbeddedLawsMatchTheWorkspace)
  - the leak test scans .agent-one/ and .claude/ too; every doc written here was checked with go test .
  - not on this machine: goreleaser and the harbor CLI — goreleaser check and the smoke run are CI's readings only

### [2026-10-11T05:45:00+02:00] orchestrator — the upgrades: the hardened gate and sandbox, the operation room, the one-page dashboard, value, the forge, evolution, themes
- **Task:** bring the engine upgrades into agent-one in its own words — the security, provider and first-run fixes and the Claude Code hooks; subagents and council, promotion, consolidation, systems, the ledger, guts, pulse, plan deck, memory review, housekeep; the sandbox's namespaces, masked credentials and Seatbelt (`--docker-agent`, `--containered` aliased); the dashboard as one page with relates and the 3D brain; the framed, Cyberpunk terminal; the value law (value/, rank/); the forge's first slice; policy.evolution; theme/ with ui.theme and ui.mode; the creed's edits to AGENT-ONE.md
- **Files:** branch `upgrades` (not pushed) — app/, board/, config/, loop/, memory/, onto/, provider/, sandbox/, tool/, tui/, workspace/, wire/, rank/, value/, theme/, AGENT-ONE.md and its two copies, README.md, CHANGELOG.md, the members' docs
- **Gate:** n/a — operator-ordered port; onto check 0 findings, housekeep whole
- **Result:** done on the branch — gofmt clean, go vet clean, go test ./... green, selftest 201, a mock turn through coordinator → domain owner → zone worker passes the gate
- **Learned:**
  - the members' docs were written with the forked-from field names (Territory, Traits), which this workspace's parser never read; they now say Owns and Invariants, and theme/, value/ and rank/ have owners
  - /dev/tcp on a command line now reads as outward, and an approved outward act gets the network: the sandbox's egress test probes from a script the gate cannot see
  - this distribution's words run longer (orchestrator, .agent-one/config.local.yaml, "to promote"): the sidebar's rank column and two notices were fitted to them
  - the unsaid's wire kinds are policy, team and domain; a prompt that says "ownership" teaches the model a token the wire drops

### [2026-10-11T12:30:00+02:00] orchestrator — the workspace in four lanes: coordinators per lane, the skills inside the workspace
- **Task:** make .agent-one/ the same shape as the engine's other distribution: one coordinator per lane instead of one for all, the skills kept in the workspace, machine-local config never tracked
- **Files:** .agent-one/coord/{engine,workspace,host,canon} (coord-agent-one became coord-canon), the six domain docs' Reports to, .agent-one/base-skills/ (the six from .claude/skills, plus loop-core, gate-safety, review-core, bench-run, zone-wire-and-journals), zone-drain wears zone-wire-and-journals, .agent-one/.gitignore (*.local.yaml); .claude/ untracked (ignored, generated)
- **Gate:** n/a — operator-ordered restructure; onto check 0 findings · 31 members · 11 skills, housekeep whole
- **Result:** done on the branch — selftest 201, go test . (the leak test) green
- **Learned:**
  - a coordinator never authors: its Owns is the lane's routing map, so a lane's dir may be both a coordinator's and a domain owner's
  - .agent-one/.gitignore predated the scaffold's config.local.* line: a token in config.local.yaml could have been committed
