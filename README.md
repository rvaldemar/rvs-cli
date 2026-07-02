# rvs CLI

Terminal client for the [RVS Agents Hub](https://hub.rvs.solutions).

```sh
curl -fsSL https://hub.rvs.solutions/cli/install.sh | sh
rvs login
rvs config show      # inspect effective API/token configuration
rvs chat              # conversational REPL (HTTP/SSE against the Hub)
rvs code              # agentic coding with laptop-local tools, brokered by the Hub
rvs task list         # Hub-issued CoS/agent tasks
rvs effort log        # log effort entries (time tracking)
rvs templates list    # list available playbook templates
rvs templates use     # instantiate a playbook template
rvs approvals list    # inspect pending or historical HITL approvals
rvs runs list         # list playbook runs for the org
rvs runs watch <id>    # watch one run to terminal
```

## Authentication

### Interactive login

```sh
rvs login
```

Opens a browser-based OAuth flow and stores the token at
`~/.config/rvs/credentials` (mode 0600).

### $RVS_TOKEN — non-interactive / CI

Set `RVS_TOKEN` in the environment and `rvs login` is skipped entirely.
Every command reads the token from the env var first; the stored credentials
file is only consulted as a fallback.

```sh
export RVS_TOKEN=rvs_cli_xxxxxxxxxxxx
rvs chat
```

Set `RVS_API_BASE` when targeting staging or a local Hub:

```sh
export RVS_API_BASE=http://localhost:3000
rvs config show
```

For a one-off override, every command also accepts `--api`:

```sh
rvs --api http://localhost:3000 config doctor
```

To generate a token: **Hub UI → Settings → CLI Tokens**
(`https://hub.rvs.solutions/settings/cli_tokens`).
Tokens are scoped per org and per user. Treat them like passwords — do not
commit them to source control.

Use `rvs config show` to confirm which API URL and token source are active.
The command always redacts the token value.

## Commands

| Command | Description |
|---|---|
| `rvs login` | Authenticate (browser OAuth or `$RVS_TOKEN`) |
| `rvs logout` | Remove stored credentials |
| `rvs config show` | Show effective API URL, token source, user/org hints, and credentials path |
| `rvs config path` | Print the credentials file path |
| `rvs config doctor` | Verify the effective token against the Hub |
| `rvs me` | Print the current authenticated user |
| `rvs chat` | Interactive chat REPL (HTTP/SSE) |
| `rvs code` | Agentic coding loop (WebSocket bridge, local tool executors) |
| `rvs list` | List recent conversations |
| `rvs task create` | Create a CoS agent task |
| `rvs task claim` | Claim the next available task |
| `rvs task run <id>` | Execute a claimed task and submit the artifact |
| `rvs effort log` | Log effort entries against tasks |
| `rvs templates list` | List available playbook templates |
| `rvs templates use` | Instantiate a playbook template into a live playbook |
| `rvs approvals list` | List approvals (`--status` supports pending/approved/rejected/expired) |
| `rvs approvals show <id>` | Show one approval detail |
| `rvs approvals decide <id> [approve|reject]` | Decide an approval and resume the playbook run |
| `rvs runs list` | List playbook runs for the org; supports `--status`, `--from`, `--to`, `--json` |
| `rvs runs show <id>` | Inspect one playbook run with step results |
| `rvs runs cancel <id>` | Cancel a running or waiting playbook run |
| `rvs runs watch <id>` | Watch a run until done/failed/cancelled |
| `rvs models` | List available LLM models |
| `rvs version` | Print CLI version |
| `rvs completion` | Generate shell completion scripts |

## Shell completions

Cobra generates completion scripts for bash, zsh, and fish.

### bash

```sh
rvs completion bash > /etc/bash_completion.d/rvs
# or for the current user only:
rvs completion bash > ~/.local/share/bash-completion/completions/rvs
```

### zsh

```sh
rvs completion zsh > "${fpath[1]}/_rvs"
```

If completion is not already enabled in your shell, add this to `~/.zshrc`:

```sh
autoload -U compinit && compinit
```

### fish

```sh
rvs completion fish > ~/.config/fish/completions/rvs.fish
```

After installing, open a new shell session (or `source` the file) for
completions to take effect.

---

`rvs code` opens a WebSocket against the Hub's `CodeBridgeChannel`, which
in turn drives the `rvs-openclaude` sidecar's QueryEngine. The LLM runs on
the Hub side; file-system tools (`Read`, `Write`, `Edit`, `Bash`, `Glob`,
`Grep`, `WebFetch`) execute locally on the laptop with TTY permission
prompts. Calls are gated and metered against the org's plan and per-user
quota. No `claude` binary required.

`rvs task` is the local runner surface for CoS-style work. The Hub owns the
`AgentTask` contract, lease and artifact; the CLI claims a task, executes only
the commands declared on that task, renews the lease, and submits the artifact
back to the Hub.

```sh
rvs task create --title "Smoke" --objective "Run a local smoke" --repo "$PWD" --cmd "true"
rvs task claim
rvs task run <task-id>
```

## Changelog

### v0.2.0

- `rvs approvals list/show/decide` — HITL approval visibility and decision workflow from terminal
- `rvs effort log` — log effort entries against tasks with duration and notes
- `rvs templates list` — list available playbook templates for the org
- `rvs templates use` — instantiate a playbook template into a live playbook
- `rvs task create/claim/run` — full CoS task runner (claim lease, exec commands, submit artifact)
- `rvs code` — gRPC/WebSocket bridge to Hub's `CodeBridgeChannel` with laptop-local tool executors
- Fix: CLI token scopes correctly set on login (CLI-01)

### v0.1.0

- `rvs login` / `rvs chat` / `rvs list` / `rvs me` / `rvs models` / `rvs version`

## Build from source

```sh
go build -o rvs .
```

## Layout

- `main.go` — version stamping + cobra entrypoint
- `cmd/` — top-level commands: `login`, `logout`, `chat`, `code`, `task`, `list`, `me`, `models`, `version`, `effort`, `templates`
- `internal/api` — HTTP client (JSON + SSE streaming) used by `rvs chat`
- `internal/config` — credentials persistence (`~/.config/rvs/credentials`, mode 0600)
- `internal/chat` — interactive REPL + slash commands
- `internal/bridge` — WebSocket subscriber for the Hub's `CodeBridgeChannel`
- `internal/tools` — local tool executors with TTY permission gate (Read/Write/Edit/Bash/Glob/Grep/WebFetch)
- `internal/openclaude/v1` + `proto/` — proto3 message definitions used over the bridge

## Releasing

Tag with `v*` (e.g. `v1.0.0`); GitHub Actions runs GoReleaser to publish:

- `rvs_linux_amd64.tar.gz`
- `rvs_linux_arm64.tar.gz`
- `rvs_darwin_amd64.tar.gz`
- `rvs_darwin_arm64.tar.gz`
- `checksums.txt`

The web installer at `https://hub.rvs.solutions/cli/install.sh` downloads from `releases/latest`.

## Tests

```sh
go test ./...
```

## License

MIT — see LICENSE.
