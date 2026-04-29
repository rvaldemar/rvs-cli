package tools

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
)

// Prompter renders a tool-call permission prompt and reads y/n from a TTY.
// Stateful: a single instance remembers per-tool "always allow" answers
// within a session ("y" once, "a" for "always for this tool").
type Prompter struct {
	in       io.Reader
	out      io.Writer
	mu       sync.Mutex
	always   map[string]bool
	closed   bool
	noTTY    bool
}

// NewPrompter wires the prompter to /dev/tty for interactive use, or falls
// back to stderr+stdin when no TTY is available (CI, piped stdin). Without
// a TTY all Prompt-class tools deny by default — there is no safe way to
// run unsupervised.
func NewPrompter() *Prompter {
	tty, err := os.OpenFile("/dev/tty", os.O_RDWR, 0)
	if err != nil {
		return &Prompter{
			in:     os.Stdin,
			out:    os.Stderr,
			always: map[string]bool{},
			noTTY:  true,
		}
	}
	return &Prompter{
		in:     tty,
		out:    tty,
		always: map[string]bool{},
	}
}

// NewPrompterWith is a test-only constructor that injects custom IO.
func NewPrompterWith(in io.Reader, out io.Writer) *Prompter {
	return &Prompter{in: in, out: out, always: map[string]bool{}}
}

// Allow asks the user whether to run the named tool with the given JSON args.
// Returns true to run, false to skip. Honors per-tool "always" answers.
//
// In no-TTY environments returns false unconditionally.
func (p *Prompter) Allow(toolName string, args json.RawMessage) bool {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.closed {
		return false
	}
	if p.always[toolName] {
		return true
	}
	if p.noTTY {
		fmt.Fprintf(p.out, "[rvs code] would run %s but no TTY is attached — denying.\n", toolName)
		return false
	}

	fmt.Fprintf(p.out, "\n[rvs code] tool: %s\n", toolName)
	if pretty := prettyArgs(args); pretty != "" {
		fmt.Fprintln(p.out, pretty)
	}
	fmt.Fprint(p.out, "approve? [y]es / [n]o / [a]lways for this tool: ")

	reader := bufio.NewReader(p.in)
	line, err := reader.ReadString('\n')
	if err != nil {
		fmt.Fprintln(p.out, "(input closed — denying)")
		p.closed = true
		return false
	}
	switch strings.ToLower(strings.TrimSpace(line)) {
	case "y", "yes":
		return true
	case "a", "always":
		p.always[toolName] = true
		return true
	default:
		return false
	}
}

func prettyArgs(args json.RawMessage) string {
	if len(args) == 0 {
		return ""
	}
	var pretty map[string]any
	if err := json.Unmarshal(args, &pretty); err != nil {
		return string(args)
	}
	out, err := json.MarshalIndent(pretty, "  ", "  ")
	if err != nil {
		return string(args)
	}
	return "  " + string(out)
}
