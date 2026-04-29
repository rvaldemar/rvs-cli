package tools

import (
	"context"
	"encoding/json"
)

// Runner ties a Registry to a Prompter so a single Execute call covers both
// permission gating and tool dispatch. Returned Result is what the caller
// sends back to the sidecar in ToolExecuteResponse.
type Runner struct {
	registry *Registry
	prompt   *Prompter
}

func NewRunner(r *Registry, p *Prompter) *Runner {
	return &Runner{registry: r, prompt: p}
}

// Execute looks up the tool by name, runs the permission gate, and invokes
// the executor if approved. If the tool isn't registered the result carries
// IsError=true so the LLM gets a clear signal.
func (r *Runner) Execute(ctx context.Context, name string, args json.RawMessage) Result {
	t, ok := r.registry.Get(name)
	if !ok {
		return Result{Output: formatError(name, errUnknownTool), IsError: true}
	}
	if t.Permission == Prompt {
		if !r.prompt.Allow(name, args) {
			return Result{Output: formatError(name, errUserDenied), IsError: true}
		}
	}
	return t.Run(ctx, args)
}

type sentinelErr string

func (e sentinelErr) Error() string { return string(e) }

const (
	errUnknownTool = sentinelErr("tool not registered on this client")
	errUserDenied  = sentinelErr("user denied permission")
)
