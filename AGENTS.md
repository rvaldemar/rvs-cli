# RVS AI Project Instructions

This project inherits the universal RVS AI operating contract from:

`~/Documents/code/.rvs/operating-contract.md`

Tool adapters:
- Codex: `~/Documents/code/.rvs/adapters/codex.md`
- Claude CLI: `~/Documents/code/.rvs/adapters/claude-cli.md`
- RVS CLI: `~/Documents/code/.rvs/adapters/rvs-cli.md`

## Local Operating Rules

- Read project docs, README files, and existing conventions before meaningful work.
- Preserve existing user/agent changes. Inspect `git status` and relevant diffs before edits.
- Use subagents/cloud when they accelerate analysis, implementation, review, or QA.
- Use the local repo as the final source of truth for diff, tests, and integration.
- Apply product, technical, security/data, operations, cost, UX, tests, diff, and release gates proportional to risk.
- Do not read or expose secret values. Use existing environment secrets blindly when needed.
- Work silently by default. Report concise results, validations, risks, blockers, and next actions.
