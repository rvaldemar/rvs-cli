package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

type bashArgs struct {
	Command     string `json:"command"`
	Description string `json:"description"`
	TimeoutMS   int    `json:"timeout"` // milliseconds; 0 → defaultBashTimeout
}

const (
	defaultBashTimeout = 2 * time.Minute
	maxBashTimeout     = 10 * time.Minute
	maxBashOutputBytes = 256 * 1024
)

func runBash(ctx context.Context, raw json.RawMessage) Result {
	var a bashArgs
	if err := json.Unmarshal(raw, &a); err != nil {
		return Result{Output: formatError("Bash", err), IsError: true}
	}
	if strings.TrimSpace(a.Command) == "" {
		return Result{Output: formatError("Bash", errors.New("command is required")), IsError: true}
	}

	timeout := time.Duration(a.TimeoutMS) * time.Millisecond
	if timeout <= 0 {
		timeout = defaultBashTimeout
	}
	if timeout > maxBashTimeout {
		timeout = maxBashTimeout
	}
	cctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd := exec.CommandContext(cctx, "bash", "-c", a.Command)
	out, err := cmd.CombinedOutput()
	if len(out) > maxBashOutputBytes {
		out = append(out[:maxBashOutputBytes], []byte(fmt.Sprintf("\n…(truncated, %d bytes total)\n", len(out)))...)
	}

	if errors.Is(cctx.Err(), context.DeadlineExceeded) {
		return Result{
			Output:  fmt.Sprintf("%s\n[bash timed out after %s]", string(out), timeout),
			IsError: true,
		}
	}
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return Result{
				Output:  fmt.Sprintf("%s\n[exit code %d]", string(out), exitErr.ExitCode()),
				IsError: true,
			}
		}
		return Result{Output: formatError("Bash", err), IsError: true}
	}
	if len(out) == 0 {
		return Result{Output: "(no output)"}
	}
	return Result{Output: string(out)}
}
