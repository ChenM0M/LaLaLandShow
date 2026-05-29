# VibeHub Codex Stop Hook

Codex project instructions load from `AGENTS.md`. For stricter stop-time enforcement, add this hook to your Codex config after confirming hooks are enabled in your environment:

```toml
[features]
codex_hooks = true

[[hooks.Stop]]
[[hooks.Stop.hooks]]
type = "command"
command = 'node "$(git rev-parse --show-toplevel)/.vibehub/adapters/hooks/vibehub-stop-check.mjs"'
timeout = 30
statusMessage = "Checking VibeHub output"
```

The hook blocks Stop when an active VibeHub task has no complete `output.md`.
