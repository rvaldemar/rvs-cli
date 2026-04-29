package tools

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func TestRunBash_Echo(t *testing.T) {
	args, _ := json.Marshal(bashArgs{Command: "echo hello"})
	res := runBash(context.Background(), args)
	if res.IsError {
		t.Fatalf("unexpected error: %s", res.Output)
	}
	if !strings.Contains(res.Output, "hello") {
		t.Errorf("expected 'hello' in output, got %q", res.Output)
	}
}

func TestRunBash_NonZeroExit(t *testing.T) {
	args, _ := json.Marshal(bashArgs{Command: "false"})
	res := runBash(context.Background(), args)
	if !res.IsError {
		t.Errorf("expected IsError on non-zero exit")
	}
	if !strings.Contains(res.Output, "exit code") {
		t.Errorf("expected exit code marker, got %q", res.Output)
	}
}

func TestRunBash_Timeout(t *testing.T) {
	args, _ := json.Marshal(bashArgs{Command: "sleep 5", TimeoutMS: 100})
	res := runBash(context.Background(), args)
	if !res.IsError {
		t.Errorf("expected IsError on timeout")
	}
	if !strings.Contains(res.Output, "timed out") {
		t.Errorf("expected timeout marker, got %q", res.Output)
	}
}

func TestRunBash_Empty(t *testing.T) {
	args, _ := json.Marshal(bashArgs{Command: ""})
	res := runBash(context.Background(), args)
	if !res.IsError {
		t.Errorf("expected IsError on empty command")
	}
}
