// Package tools holds the local executors that the laptop client offers to
// the openclaude sidecar via the gRPC bridge. The sidecar's QueryEngine sees
// these as MCP-shaped external tools (name + JSON schema) and round-trips
// every call through the bridge: tool_execute → ToolExecuteResponse.
//
// We deliberately mirror the Anthropic Claude Code built-in tool names and
// schemas (Read/Write/Edit/Bash/Glob/Grep/WebFetch) so the LLM, which was
// trained on those, uses them naturally. The Hub's CodeBridgeChannel injects
// `disabled_builtin_tools` so the sidecar suppresses its own copies and the
// LLM only sees the laptop-delegated ones.
package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
)

// PermissionMode controls whether a tool runs unattended or asks the user.
type PermissionMode int

const (
	// AutoApprove runs without confirmation. Reserved for read-only tools.
	AutoApprove PermissionMode = iota
	// Prompt asks the user via TTY before executing.
	Prompt
)

// Tool is a single laptop-side executor.
type Tool struct {
	Name        string
	Description string
	// InputSchema is a JSON Schema document (object). Forwarded verbatim to
	// the sidecar in ChatRequest.tools[].input_schema_json.
	InputSchema map[string]any
	Permission  PermissionMode
	Run         func(ctx context.Context, args json.RawMessage) Result
}

// Result is what a Tool.Run returns. Output is plain text shown to the LLM.
// IsError flags execution failures (the sidecar reports back to the LLM as
// a tool_result with isError=true).
type Result struct {
	Output  string
	IsError bool
}

// Definition is the proto-friendly view of a Tool. Used to populate
// ChatRequest.tools[].
type Definition struct {
	Name            string
	Description     string
	InputSchemaJSON string
}

// Registry is an ordered, name-keyed collection of Tools.
type Registry struct {
	tools map[string]Tool
	order []string
}

// NewRegistry returns the default rvs-cli laptop registry: Read, Write, Edit,
// Bash, Glob, Grep, WebFetch.
func NewRegistry() *Registry {
	r := &Registry{tools: map[string]Tool{}}
	for _, t := range defaultTools() {
		r.add(t)
	}
	return r
}

// Empty returns a registry with no tools. Used in tests.
func Empty() *Registry {
	return &Registry{tools: map[string]Tool{}}
}

func (r *Registry) add(t Tool) {
	if _, exists := r.tools[t.Name]; !exists {
		r.order = append(r.order, t.Name)
	}
	r.tools[t.Name] = t
}

// Register adds or replaces a tool. Visible for tests.
func (r *Registry) Register(t Tool) { r.add(t) }

// Get returns the tool with the given name. ok=false if not registered.
func (r *Registry) Get(name string) (Tool, bool) {
	t, ok := r.tools[name]
	return t, ok
}

// Names lists the registered tool names in insertion order.
func (r *Registry) Names() []string {
	out := make([]string, len(r.order))
	copy(out, r.order)
	return out
}

// Definitions returns the proto-shaped tool list to put on ChatRequest.tools.
// Order is deterministic (insertion order, with a stable fallback sort) so
// the sidecar sees the same tool list across runs.
func (r *Registry) Definitions() ([]Definition, error) {
	names := append([]string{}, r.order...)
	sort.SliceStable(names, func(i, j int) bool { return names[i] < names[j] })

	defs := make([]Definition, 0, len(names))
	for _, n := range names {
		t := r.tools[n]
		schemaBytes, err := json.Marshal(t.InputSchema)
		if err != nil {
			return nil, fmt.Errorf("marshal schema for %s: %w", t.Name, err)
		}
		defs = append(defs, Definition{
			Name:            t.Name,
			Description:     t.Description,
			InputSchemaJSON: string(schemaBytes),
		})
	}
	return defs, nil
}

// DisabledBuiltinNames is the canonical list of sidecar built-in tools the
// laptop replaces. Returned in the same order CodeBridgeChannel injects them
// (Hub-side hardening will overwrite this if the client sends an empty list,
// but we send it explicitly to make protocol intent clear in transit).
func (r *Registry) DisabledBuiltinNames() []string {
	out := append([]string{}, r.order...)
	sort.Strings(out)
	return out
}

// formatError renders a tool failure as a stable, LLM-friendly string. The
// LLM sees this in tool_result; the wrapper makes it grep-able and avoids
// leaking Go-flavored error text.
func formatError(toolName string, err error) string {
	return fmt.Sprintf("tool %s failed: %v", toolName, err)
}
