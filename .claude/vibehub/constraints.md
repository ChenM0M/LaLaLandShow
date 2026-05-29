# VibeHub Constraints for Claude Code

Adapter template: 2.0.0-pre.3
Skills registry: missing (.vibehub/skills.registry.yaml)

Read these files before VibeHub-scoped work:
- `.vibehub/agent-view/current.md`
- `.vibehub/agent-view/current-context.md`
- `.vibehub/agent-view/handoff.md`
- `.vibehub/rules/hard-rules.md`
- `.vibehub/adapters/protocol.md`

Claude Code may inspect and change project files within the active task scope.
Claude Code must not edit `.vibehub/state.yaml`, current task/run pointer files, or mark canonical state complete.

Use `.claude/commands/vibehub-*.md slash commands` for VibeHub operations.
Use `CLAUDE.md` as the automatic static protocol entry.

Before ending work on an active VibeHub task, write the phase output file required by `.vibehub/adapters/protocol.md`.

If work happened outside VibeHub, use `vibehub-sync` and return a sync report.
If state or Git HEAD drifted, run `vibehub-recover` and return a recover report.

Adapter writes are the only INV-6 UI-write exception and are limited to generated adapter configuration files.
