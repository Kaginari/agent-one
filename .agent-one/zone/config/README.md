# zone-config

- **Rank:** Zone worker
- **Territory:** `config/cli.go`, `config/config_test.go`, `config/decode.go`, `config/defaults.go`, `config/dist.go`, `config/explain.go`, `config/load.go`, `config/models.go`, `config/patch.go`, `config/patch_test.go`, `config/permissions.go`, `config/permissions_test.go`, `config/ranks.go`, `config/rules.go`, `config/schema.go`, `config/tools.go`, `config/tools_models_test.go`, `config/validate.go`
- **Reports to:** domain-config
- **Skills:** zone-go-package
- **Purpose:** the ground truth of the switchboard's own files: the schema, the layers, the origins, the refusals, models, permissions, ranks, rules and the registry.

## Traits
- `Config` (schema.go) is decoded by reflection from the merged `yaml.Node` tree (decode.go); `Origins` maps every dotted key to file:line for `config explain`. `[rank-dirs]` in a default path expands to `Dist.RankDirs` = "coord,domain,zone" — service is not in it.
- Models: `models.default`, roles analyst · judge · drafter, ranks, members, tasks drain · gate · log · bench; `ResolveModel(member, rank, role, task)` — the most specific wins; a fallback is tried on 429/5xx and transport errors, never on a refusal. The shipped default is `anthropic/claude-opus-5`; the registry gives a model its tier, contextWindow and price per 1M tokens.
- Permissions: `<tool>:<glob>` rules with allow · ask · deny; specificity beats order; Deny > Ask > Allow; `question` is an alias of `ask`; an unknown tool name is a hole; `Loosenings()` lists every silenced question for `config check`.
- Ranks: `builtinRanks` coord · domain · zone · service · principal-coordinator · principal-domain-owner · auditor; `rankSet: extend|replace` with `ranks:` definitions; a prefix must be its dir + "-". Rules: text plus an optional `check` command scoped all · rank:<r> · member:<m>, run at the gate.
- Registry (`registry.yaml`): models, tools (externals for the toolbox), packages (npm/pip/go mirrors, `tokenEnv` let through by name, `env`), containers (base, apt, image); `mergeRegistry` folds it in; `Packages.PackageEnv()` feeds the sandbox's EnvSet.
- `Duration` accepts "2m" or plain seconds; `AutoInt` accepts "auto"; `foldTimeouts` normalises the old timeout keys. `config.CLI`: show [--yaml] · explain · check · path · patch; `SelftestIn` counts its checks into the binary's selftest.

## Verify
- `go test ./config/`

## Working notes
