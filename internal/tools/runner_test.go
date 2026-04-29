package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func TestRunner_UnknownTool(t *testing.T) {
	r := Empty()
	p := NewPrompterWith(strings.NewReader(""), &bytes.Buffer{})
	res := NewRunner(r, p).Execute(context.Background(), "Nope", json.RawMessage("{}"))
	if !res.IsError {
		t.Errorf("expected IsError for unknown tool")
	}
}

func TestRunner_AutoApproveSkipsPrompt(t *testing.T) {
	r := Empty()
	r.Register(Tool{
		Name:       "Read",
		Permission: AutoApprove,
		Run: func(_ context.Context, _ json.RawMessage) Result {
			return Result{Output: "ran"}
		},
	})
	// Prompter input is empty — would deny if asked.
	out := &bytes.Buffer{}
	p := NewPrompterWith(strings.NewReader(""), out)
	res := NewRunner(r, p).Execute(context.Background(), "Read", json.RawMessage("{}"))
	if res.IsError || res.Output != "ran" {
		t.Errorf("auto-approve tool should run without prompt: %+v", res)
	}
	if out.Len() > 0 {
		t.Errorf("auto-approve should not have prompted")
	}
}

func TestRunner_PromptApproved(t *testing.T) {
	r := Empty()
	r.Register(Tool{
		Name:       "Bash",
		Permission: Prompt,
		Run: func(_ context.Context, _ json.RawMessage) Result {
			return Result{Output: "ran"}
		},
	})
	out := &bytes.Buffer{}
	p := NewPrompterWith(strings.NewReader("y\n"), out)
	res := NewRunner(r, p).Execute(context.Background(), "Bash", json.RawMessage(`{"command":"ls"}`))
	if res.IsError || res.Output != "ran" {
		t.Errorf("expected ran, got %+v", res)
	}
}

func TestRunner_PromptDenied(t *testing.T) {
	r := Empty()
	r.Register(Tool{
		Name:       "Bash",
		Permission: Prompt,
		Run: func(_ context.Context, _ json.RawMessage) Result {
			return Result{Output: "ran"}
		},
	})
	p := NewPrompterWith(strings.NewReader("n\n"), &bytes.Buffer{})
	res := NewRunner(r, p).Execute(context.Background(), "Bash", json.RawMessage(`{}`))
	if !res.IsError {
		t.Errorf("expected IsError on denial")
	}
	if !strings.Contains(res.Output, "denied") {
		t.Errorf("expected denial marker, got %q", res.Output)
	}
}

func TestRunner_PromptAlwaysSticks(t *testing.T) {
	r := Empty()
	calls := 0
	r.Register(Tool{
		Name:       "Bash",
		Permission: Prompt,
		Run: func(_ context.Context, _ json.RawMessage) Result {
			calls++
			return Result{Output: "ran"}
		},
	})
	out := &bytes.Buffer{}
	p := NewPrompterWith(strings.NewReader("a\n"), out)
	runner := NewRunner(r, p)

	if res := runner.Execute(context.Background(), "Bash", json.RawMessage(`{}`)); res.IsError {
		t.Fatalf("first call should be approved: %+v", res)
	}
	// Second call: input is empty — prompter would deny — but `always` should
	// short-circuit the prompt.
	if res := runner.Execute(context.Background(), "Bash", json.RawMessage(`{}`)); res.IsError {
		t.Errorf("always-allow should skip subsequent prompts: %+v", res)
	}
	if calls != 2 {
		t.Errorf("expected 2 calls, got %d", calls)
	}
}

func TestRegistry_Definitions_Stable(t *testing.T) {
	r := NewRegistry()
	defs, err := r.Definitions()
	if err != nil {
		t.Fatal(err)
	}
	wantNames := []string{"Bash", "Edit", "Glob", "Grep", "Read", "WebFetch", "Write"}
	if len(defs) != len(wantNames) {
		t.Fatalf("expected %d defs, got %d", len(wantNames), len(defs))
	}
	for i, want := range wantNames {
		if defs[i].Name != want {
			t.Errorf("position %d: got %q, want %q", i, defs[i].Name, want)
		}
		if defs[i].InputSchemaJSON == "" {
			t.Errorf("schema for %s should not be empty", defs[i].Name)
		}
	}
}

func TestRegistry_DisabledBuiltinNames(t *testing.T) {
	r := NewRegistry()
	got := r.DisabledBuiltinNames()
	want := []string{"Bash", "Edit", "Glob", "Grep", "Read", "WebFetch", "Write"}
	if len(got) != len(want) {
		t.Fatalf("expected %d names, got %d (%v)", len(want), len(got), got)
	}
	for i, w := range want {
		if got[i] != w {
			t.Errorf("position %d: got %q, want %q", i, got[i], w)
		}
	}
}
