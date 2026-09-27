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
