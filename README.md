# rvs CLI

Terminal client for the [RVS Agents Hub](https://agents.rvs.solutions).

```sh
curl -fsSL https://agents.rvs.solutions/cli/install.sh | sh
rvs login
rvs chat              # conversational REPL (HTTP/SSE against the Hub)
rvs code              # agentic coding with laptop-local tools, brokered by the Hub
```

`rvs code` opens a WebSocket against the Hub's `CodeBridgeChannel`, which
in turn drives the `rvs-openclaude` sidecar's QueryEngine. The LLM runs on
the Hub side; file-system tools (`Read`, `Write`, `Edit`, `Bash`, `Glob`,
`Grep`, `WebFetch`) execute locally on the laptop with TTY permission
prompts. Calls are gated and metered against the org's plan and per-user
quota. No `claude` binary required.

## Build from source

```sh
go build -o rvs .
```

## Layout

- `main.go` — version stamping + cobra entrypoint
- `cmd/` — top-level commands: `login`, `logout`, `chat`, `code`, `list`, `me`, `models`, `version`
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
