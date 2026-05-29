---
name: vibehub-claim
description: "Claim a named capability when gates allow it. Use for VibeHub workflow step: vibehub-claim."
---

# vibehub-claim

中文: 在 gate 允许时 claim 指定 capability。
English: Claim a named capability when gates allow it.

Invocation input: <capability>

Read first:
- `.vibehub/agent-view/current.md`
- `.vibehub/agent-view/current-context.md`
- `.vibehub/agent-view/handoff.md`
- `.vibehub/rules/hard-rules.md`
- `.vibehub/adapters/protocol.md`

Task:
Claim a named capability only after VibeHub gates allow it. Prefer `claim <project_path> <capability>` or `vibehub claim <project_path> <capability>` when the CLI is available, then read the rebuilt capability context pack and continue the work.



Output requirements:
- write the active run phase output before ending work:
  `.vibehub/tasks/<task_id>/runs/<run_id>/outputs/output.md`
- changed files, if any
- files read
- commands run
- tests run, or reason not run
- evidence labels: `hard_observed`, `agent_reported`, `inferred`, `user_confirmed`
- unresolved risks
- handoff notes or recommended VibeHub action

Constraints:
- Do not edit `.vibehub/state.yaml` or canonical task/run pointers directly.
- Do not claim runtime observation unless a runtime adapter captured it.
- If state is stale or drifted, report it and recommend VibeHub sync/recover instead of silently advancing state.
