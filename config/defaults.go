package config

// defaultsJSON is layer 1, one table for both distributions; [dist] and [dist-dir] are
// substituted by the lexicon at decode. Every feature object carries `enabled`.
//
// dispatch.sessionBackground exists as a per-workspace flip (config.local.yaml) for sessions that
// want their dispatches woken: the wake channel feeds the session only, a mid-chain subagent's
// background dispatch would strand its report. Foreground stays the default everywhere.
const defaultsJSON = `{
  "models": {
    "default": "anthropic/claude-opus-5",
    "roles": {},
    "ranks": {},
    "members": {},
    "tasks": {}
  },
  "smallModel": "",
  "mode": "build",
  "providers": {
    "anthropic":  { "enabled": true,  "type": "anthropic", "apiKeyEnv": "ANTHROPIC_API_KEY", "baseURL": "https://api.anthropic.com", "maxOutputTokens": 8192, "timeout": "10m", "toolCalls": "native", "contextWindow": "auto", "thinking": "adaptive", "fallbacks": "default", "models": {} },
    "openai":     { "enabled": true,  "type": "openai", "apiKeyEnv": "OPENAI_API_KEY", "baseURL": "https://api.openai.com/v1", "timeout": "10m", "toolCalls": "native", "contextWindow": "auto", "models": {} },
    "openrouter": { "enabled": false, "type": "openai", "apiKeyEnv": "OPENROUTER_API_KEY", "baseURL": "https://openrouter.ai/api/v1", "timeout": "10m", "toolCalls": "native", "contextWindow": "auto", "models": {} },
    "ollama":     { "enabled": false, "type": "openai", "apiKeyEnv": "", "baseURL": "http://127.0.0.1:11434/v1", "timeout": "10m", "toolCalls": "native", "contextWindow": "auto", "models": {} },
    "mock":       { "enabled": false, "type": "mock", "script": "" }
  },
  "guard": { "enabled": true, "files": [], "patterns": [] },
  "registry": { "models": {}, "tools": [], "packages": { "npm": "", "pip": "", "go": "", "tokenEnv": "", "env": {} }, "containers": { "image": "", "base": "", "apt": "" } },
  "tools": {
    "profile":   "max",
    "read":      { "enabled": true, "maxLines": 2000, "maxBytes": 51200 },
    "ls":        { "enabled": true, "limit": 500 },
    "glob":      { "enabled": true, "limit": 100 },
    "grep":      { "enabled": true, "limit": 100 },
    "write":     { "enabled": true },
    "edit":      { "enabled": true },
    "delete":    { "enabled": true },
    "multiedit": { "enabled": true },
    "patch":     { "enabled": true },
    "bash":      { "enabled": true, "shell": "", "timeout": "2m", "maxTimeout": "10m", "background": true, "jobsDir": "[dist-dir]/tmp/jobs", "sandbox": "bwrap", "envAllow": [] },
    "git":       { "enabled": true, "timeout": "2m" },
    "webfetch":  { "enabled": true, "maxBytes": 5242880, "timeout": "1m" },
    "websearch": { "enabled": true, "timeout": "1m", "backend": "" },
    "ask":       { "enabled": true },
    "dispatch":  { "enabled": true, "maxDepth": 3, "background": false, "backgroundTimeout": "30m", "strictFlow": true, "lineage": true, "sessionBackground": false },
    "recall":    { "enabled": true },
    "remember":  { "enabled": true },
    "toolbox":   { "enabled": true },
    "onto":      { "enabled": true },
    "skill":     { "enabled": true },
    "workingNotes":       { "enabled": true, "limit": 5 },
    "plan":       { "enabled": true },
    "pulse":      { "enabled": true },
    "queue":      { "enabled": true },
    "fold":       { "enabled": true },
    "custom":    {},
    "output":    { "maxLines": 2000, "maxBytes": 51200, "dumpDir": "[dist-dir]/tmp/tool-out", "keepDays": 7 },
    "missing":   { "doomLoopRepeats": 2, "proposeOnSecondNaming": true, "liveReload": true },
    "tmpRedirect": { "enabled": true, "mode": "rewrite", "refuseCwd": true },
    "preferFileTools": "hint"
  },
  "policy": {
    "principles":      { "enabled": true },
    "humanGate":  { "enabled": true, "strict": false, "approve": [], "dryRun": false, "trustContainer": false },
    "gate":       { "enabled": true, "rightAuthor": true, "invariantsHold": true, "dutiesDone": true, "docTruthful": true, "retries": 1, "testsIntact": true, "ui": true, "verifyTimeout": "120s" },
    "docsAsCode":   { "enabled": true },
    "ownership":  { "enabled": true, "orchestratorNeverAuthors": true },
    "wire":       { "enabled": true, "cap": 2048, "requireUnsaid": true, "raw": false },
    "log":        { "enabled": true, "path": "[dist-dir]/log.md" },
    "escalation": { "enabled": true, "maxRetries": 2 },
    "budget":     { "contextTokens": 200000, "stressTokens": 180000, "steps": 40, "minutes": 60, "salvageTokens": 4096 },
    "guts":       { "enabled": true, "window": 5, "floor": -2, "benchThrashVisits": 6, "digestEvery": 10, "ring": 12, "repin": 3, "actBudget": 40 },
    "value":      { "enabled": true, "window": "28d", "pass": ["DONE", "PASS"], "fail": ["FAIL", "ESCALATE", "CHECKPOINT"], "neutral": ["DENIED", "INTERRUPTED", "DRY", "SUSPENDED"], "reopenTurns": 3, "minRecord": 5, "families": {} },
    "doomLoop":   { "enabled": true, "threshold": 3 },
    "stress":     { "escalateAt": 5, "maxConsults": 3, "relieveAt": 7 },
    "forge":      { "demandAtSteps": 12, "evolvedStatus": "trial" },
    "evolution":  { "enabled": true, "mode": "auto", "notesMax": 7, "stressAt": 0, "failStreak": 3, "cooldownRuns": 10, "arms": ["split", "mutate", "rebirth"], "splitLines": 1500, "splitFiles": 24 },
    "promote":    { "greenFloor": 0, "tenure": 0 }
  },
  "memory": {
    "short":  { "enabled": true, "cacheDir": "[dist-dir]/memory/short" },
    "long":   { "enabled": true, "index": "[dist-dir]/memory/long/index.json", "rebuildOnStale": true, "indexOnBoot": true },
    "shared": { "workspace":   { "enabled": true, "path": "[dist-dir]/memory/shared/notes.jsonl" },
                "machine": { "enabled": true, "path": "~/.[dist]/shared/notes.jsonl" } },
    "recall": { "k": 5, "relationBoost": true }
  },
  "toolbox":  { "enabled": true, "budgetTokens": 1500, "registry": "[dist-dir]/toolbox/registry.json", "extra": "[dist-dir]/toolbox/extra.jsonl" },
  "ontology": { "enabled": true, "schema": "[dist-dir]/ontology/schema.ttl", "graphDir": "[dist-dir]/ontology/graph", "projectBudgetTokens": 800, "validate": true },
  "instruments": {
    "context": { "enabled": true },
    "loop":    { "enabled": true, "journalDir": "[dist-dir]/instruments/loop" },
    "toolbox": { "enabled": true, "journalDir": "[dist-dir]/instruments/toolbox" },
    "status":  { "showOff": true }
  },
  "permissions": {
    "enabled": true,
    "rules": [],
    "import": { "claudeCode": { "enabled": true }, "opencode": { "enabled": true } }
  },
  "rules": [],
  "hooks": { "enabled": true, "timeout": "10s", "preTool": [], "postTool": [], "sessionStart": [], "preCompact": [], "stop": [], "userPrompt": [] },
  "mcp": {
    "enabled": true,
    "timeout": "5s",
    "import": { "claudeCode": { "enabled": true, "path": ".mcp.json" }, "opencode": { "enabled": true } },
    "servers": {}
  },
  "discovery": {
    "instructions": { "enabled": true, "files": ["AGENTS.md", "CLAUDE.md"],
      "global": ["~/.config/[dist]/AGENTS.md", "~/.config/opencode/AGENTS.md", "~/.claude/CLAUDE.md"], "walkUp": true },
    "skills":   { "enabled": true, "paths": ["[dist-dir]/crafted-skills", "[dist-dir]/base-skills", "[dist-dir]/skills", ".claude/skills", ".opencode/skills", ".opencode/skill", ".agents/skills",
                  "~/.config/[dist]/skills", "~/.claude/skills", "~/.config/opencode/skills", "~/.config/opencode/skill"] },
    "commands": { "enabled": true, "paths": ["[dist-dir]/commands", ".claude/commands", ".opencode/commands", ".opencode/command",
                  "~/.config/[dist]/commands", "~/.claude/commands", "~/.config/opencode/commands", "~/.config/opencode/command"] },
    "agents":   { "enabled": true, "paths": ["[dist-dir]/{[rank-dirs]}", ".claude/agents", ".opencode/agents", ".opencode/agent",
                  "~/.config/[dist]/agents", "~/.claude/agents", "~/.config/opencode/agents", "~/.config/opencode/agent"] }
  },
  "sessions": { "enabled": true, "dir": "~/.local/share/[dist]/sessions", "keepDays": 30, "title": true },
  "compaction": {
    "enabled": true,
    "strategy": "drain",
    "trigger": { "tokens": 180000, "fraction": 0.85 },
    "keepRecentTurns": 4,
    "passes": { "pointerize": { "enabled": true }, "trimSpent": { "enabled": true }, "unsaid": { "enabled": true },
                "workingNotes": { "enabled": true }, "episode": { "enabled": true }, "verify": { "enabled": true } },
    "journal": true
  },
  "plan":   { "enabled": true, "dir": "[dist-dir]/tmp/plans" },
  "undo":   { "enabled": true, "dir": "~/.local/share/[dist]/undo", "keepDays": 7 },
  "output": { "format": "text", "stream": true, "thinking": false, "color": "auto" },
  "budgets": { "session": { "tokens": 0, "usd": 0 }, "subagent": { "tokens": 0, "usd": 0 } },
  "ui": { "statusLine": true, "announceSubagents": true, "theme": "cyberpunk", "mode": "dark", "board": { "autostart": true, "port": 7411 }, "meters": {} },
  "logLevel": "WARN"
}`
