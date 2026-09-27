# Example: agent-one on vLLM-hosted models behind a gateway

One OpenAI-compatible gateway in front of the models; the configuration split into one file per part.

```
.agent-one/
├── config.yaml        # the workspace's own settings (budgets, the dashboard)
├── providers.yaml     # the gateway: its URL, the key's variable name, native tool calls
├── registry.yaml      # the models the gateway serves, tools, the private Artifactory, the container registry
├── models.yaml        # who runs on what: the session and the three roles, each with a fallback
├── guards.yaml        # catastrophic commands refused before any approval (added to the built-in list)
├── rules.yaml         # rules every agent reads; a rule with a check runs at the review gate
└── permissions.yaml   # ask / allow / deny per command
```

To use it in a project:

```sh
cp -r examples/gateway/.agent-one your-project/     # then edit providers.yaml's baseURL
cd your-project && agent-one init                   # adds the policy and the workspace dirs; keeps your config
export GATEWAY_API_KEY=…
agent-one status        # every model resolved, with the file each came from
agent-one config explain
agent-one               # the session
```

The model IDs are placeholders — use the names the gateway's `/v1/models` lists.
