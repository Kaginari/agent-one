# domain-config

- **Rank:** Domain owner
- **Owns:** `config/`
- **Reports to:** coord-workspace
- **Skills:** domain-gate-review
- **Purpose:** rules the switchboard — every layer, every origin, every refusal — and holds its gate: no key is added, no default flipped, no refusal weakened without this owner's pass.

## Invariants
- Layers in precedence order: built-in defaults (defaults.go, JSON) → global `~/.config/agent-one/` → project `.agent-one/` → project local `config.local.*` → `AGENT_ONE_CONFIG` / `AGENT_ONE_CONFIG_CONTENT` → env keys → flags. Section files beside config.yaml (`providers.yaml`, `models.yaml`, `guards.yaml` → guard, …) fold into their layer and win over it; two files for one section is an error.
- A credential in a config file is refused: a key named apikey, key, token, secret, password or auth under `providers.*` or `mcp.*`; keys are named by `apiKeyEnv`. An `allow` with a wildcard-only pattern on an outward-capable tool is refused as an auto-approve mode.
- `enabled: false` under policy, memory, instruments, toolbox, ontology, compaction, hooks, permissions, mcp or discovery is a finding `status` prints (`Off`), unless the default shipped it off.
- Unknown keys become `@?` holes, never errors — except the secret-shaped ones above.
- The newest sections: the local layer's scaffold (`config scaffold`), ui.theme and ui.mode, policy.value (families, scores, meters), policy.evolution, policy.guts and policy.forge — each with its `config explain` view and its off-switch reported by `status`.
- Zone workers: zone-config (the config package's own files), zone-yaml (config/yaml/).

## Verify
- `go test ./config/...`

## Working notes
