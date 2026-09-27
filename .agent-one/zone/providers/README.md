# zone-providers

- **Rank:** Zone worker
- **Territory:** `provider/`
- **Reports to:** domain-engine
- **Skills:** zone-go-package
- **Purpose:** the ground truth of the one standing outward act — a chat turn with a model: the shared request shape and the two vendor dialects plus the mock.

## Traits
- anthropic: the key comes from `ANTHROPIC_API_KEY` only (the error says "location only: the key never crosses the wire"); `DefaultModel` is `claude-opus-5`, API version `2023-06-01`; `New` switches on adaptive thinking, refusal fallbacks (beta `server-side-fallback-2026-07-01`), `cache_control` on the system prefix, streaming, and `max_tokens` 16000. A tool with a vendor `Declare` (bash_20250124, text_editor_20250728) is sent as that object instead of its schema.
- openai: any chat-completions endpoint (vLLM, Ollama, OpenRouter, a Gemini compat layer); an empty API key is allowed. Retries 429/500/502/503/504 three times honouring `Retry-After` (≤ 60 s) else 1 s, 2 s, 4 s. `ToolCalls: "text"` parses `<tool_call>{…}</tool_call>` tags for models without native tool calling; `ContextWindow` reads /v1/models once (`max_model_len`, `context_length`, `context_window`).
- `provider.Validate` enforces the shape every vendor needs: roles alternate, tool results answer exactly the preceding calls, the conversation ends on a user message. A failure here is the caller's bug, not the vendor's.
- `AGENT_ONE_MODEL` overrides a provider's default model (`provider.DefaultModel`); `provider.Getenv` is a variable tests swap.
- mock is the scripted provider tests and the selftest use (`mock.Text`, `mock.Call`, `mock.Calls`).
- Hazard: the HTTP client timeout is 10 minutes a call; a provider's `Headers` map is trusted here and refused upstream by config when it looks like a credential.

## Verify
- `go test ./provider/...`

## Working notes
