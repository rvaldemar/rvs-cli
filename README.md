# rvs CLI

Terminal client for the [RVS Agents Hub](https://agents.rvs.solutions).

```sh
curl -fsSL https://agents.rvs.solutions/cli/install.sh | sh
rvs login
rvs chat              # conversational REPL (HTTP/SSE against the Hub)
rvs code              # agentic coding with laptop-local tools, brokered by the Hub
rvs task list         # Hub-issued CoS/agent tasks
rvs effort log        # log effort entries (time tracking)
rvs templates list    # list available playbook templates
rvs templates use     # instantiate a playbook template
```

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

The web installer at `https://agents.rvs.solutions/cli/install.sh` downloads from `releases/latest`.

## Tests

```sh
go test ./...
```

## License

MIT — see LICENSE.
