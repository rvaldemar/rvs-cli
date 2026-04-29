# rvs CLI

Terminal client for the [RVS Agents Hub](https://agents.rvs.solutions).

```sh
curl -fsSL https://agents.rvs.solutions/cli/install.sh | sh
rvs login
rvs chat              # conversational REPL
rvs code              # Anthropic Claude Code, routed through the Hub
```

`rvs code` requires the `claude` binary on PATH
(`npm install -g @anthropic-ai/claude-code`). It runs Claude Code with
`ANTHROPIC_BASE_URL` pointing at the Hub's passthrough, so all calls go
through the org's quota and budget tracking.

## Build from source

```sh
go build -o rvs .
```

## Layout

- `main.go` — version stamping + cobra entrypoint
- `cmd/` — top-level commands: `login`, `logout`, `chat`, `code`, `list`, `me`, `models`, `version`
- `internal/api` — HTTP client (JSON + SSE streaming)
- `internal/config` — credentials persistence (`~/.config/rvs/credentials`, mode 0600)
- `internal/chat` — interactive REPL + slash commands

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
