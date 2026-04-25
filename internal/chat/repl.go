// Package chat implements the interactive REPL.
//
// The REPL reads one line at a time from stdin. Lines starting with `/`
// are slash-commands; otherwise the line is sent as a user message and
// the assistant reply is streamed inline.
package chat

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"text/tabwriter"

	"github.com/rvaldemar/rvs-openclaude/cli/internal/api"
)

type Session struct {
	Client    *api.Client
	APIBase   string
	UserEmail string
	Conv      *api.Conversation
	Stdout    io.Writer
	Stderr    io.Writer
}

func New(c *api.Client, apiBase, userEmail string) *Session {
	return &Session{
		Client:    c,
		APIBase:   apiBase,
		UserEmail: userEmail,
		Stdout:    os.Stdout,
		Stderr:    os.Stderr,
	}
}

func (s *Session) Run(ctx context.Context) error {
	ctx, cancel := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer cancel()

	fmt.Fprintln(s.Stdout, "rvs chat — type /help for commands, /exit to quit.")
	if s.Conv == nil {
		conv, err := s.Client.CreateConversation(ctx, "")
		if err != nil {
			return fmt.Errorf("create conversation: %w", err)
		}
		s.Conv = conv
		fmt.Fprintf(s.Stdout, "New conversation %s\n", conv.ID)
	}

	reader := bufio.NewReader(os.Stdin)
	for {
		s.prompt()
		line, err := reader.ReadString('\n')
		if err != nil {
			if errors.Is(err, io.EOF) {
				fmt.Fprintln(s.Stdout)
				return nil
			}
			return err
		}
		line = strings.TrimRight(line, "\r\n")
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "/") {
			done, err := s.handleCommand(ctx, line)
			if err != nil {
				fmt.Fprintln(s.Stderr, "error:", err)
			}
			if done {
				return nil
			}
			continue
		}
		if err := s.sendMessage(ctx, line); err != nil {
			var qe *api.QuotaError
			if errors.As(err, &qe) {
				fmt.Fprintln(s.Stderr, "quota exceeded:", qe.Message)
				return nil
			}
			fmt.Fprintln(s.Stderr, "error:", err)
		}
	}
}

func (s *Session) prompt() {
	convLabel := "no-conv"
	if s.Conv != nil {
		convLabel = s.Conv.ID[:8]
	}
	fmt.Fprintf(s.Stdout, "[%s]> ", convLabel)
}

func (s *Session) sendMessage(ctx context.Context, content string) error {
	fmt.Fprint(s.Stdout, "assistant: ")
	streamCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	err := s.Client.StreamMessage(streamCtx, s.Conv.ID, content, func(ev api.StreamEvent) error {
		switch ev.Type {
		case "chunk":
			var payload struct {
				Delta string `json:"delta"`
			}
			if err := json.Unmarshal(ev.Data, &payload); err == nil {
				fmt.Fprint(s.Stdout, payload.Delta)
			}
		case "done":
			fmt.Fprintln(s.Stdout)
		case "error":
			var payload struct {
				Code, Message string
			}
			_ = json.Unmarshal(ev.Data, &payload)
			fmt.Fprintf(s.Stderr, "\n[stream error: %s] %s\n", payload.Code, payload.Message)
		}
		return nil
	})
	return err
}

// === Slash commands ===

func (s *Session) handleCommand(ctx context.Context, line string) (bool, error) {
	parts := strings.SplitN(line, " ", 2)
	cmd := parts[0]
	arg := ""
	if len(parts) == 2 {
		arg = strings.TrimSpace(parts[1])
	}

	switch cmd {
	case "/exit", "/quit":
		return true, nil
	case "/help":
		s.printHelp()
		return false, nil
	case "/clear":
		fmt.Fprint(s.Stdout, "\033[H\033[2J")
		return false, nil
	case "/new":
		conv, err := s.Client.CreateConversation(ctx, arg)
		if err != nil {
			return false, err
		}
		s.Conv = conv
		fmt.Fprintf(s.Stdout, "New conversation %s\n", conv.ID)
	case "/list":
		convs, err := s.Client.ListConversations(ctx)
		if err != nil {
			return false, err
		}
		w := tabwriter.NewWriter(s.Stdout, 0, 4, 2, ' ', 0)
		fmt.Fprintln(w, "ID\tUPDATED\tTITLE")
		for _, c := range convs {
			title := c.Title
			if title == "" {
				title = "(untitled)"
			}
			fmt.Fprintf(w, "%s\t%s\t%s\n", c.ID[:8], c.UpdatedAt, title)
		}
		_ = w.Flush()
	case "/open":
		if arg == "" {
			return false, errors.New("/open <id>")
		}
		s.Conv = &api.Conversation{ID: arg}
		fmt.Fprintf(s.Stdout, "Switched to %s\n", arg)
	case "/rename":
		if arg == "" {
			return false, errors.New("/rename <new title>")
		}
		if err := s.Client.RenameConversation(ctx, s.Conv.ID, arg); err != nil {
			return false, err
		}
		s.Conv.Title = arg
		fmt.Fprintln(s.Stdout, "Renamed.")
	case "/delete":
		id := s.Conv.ID
		if err := s.Client.DeleteConversation(ctx, id); err != nil {
			return false, err
		}
		fmt.Fprintf(s.Stdout, "Deleted %s. Starting a new one.\n", id)
		conv, err := s.Client.CreateConversation(ctx, "")
		if err != nil {
			return false, err
		}
		s.Conv = conv
	case "/export":
		format := arg
		if format == "" {
			format = "markdown"
		}
		if format == "md" {
			format = "markdown"
		}
		body, err := s.Client.ExportConversation(ctx, s.Conv.ID, format)
		if err != nil {
			return false, err
		}
		fmt.Fprintln(s.Stdout, body)
	case "/tier":
		q, err := s.Client.Quota(ctx)
		if err != nil {
			return false, err
		}
		fmt.Fprintf(s.Stdout, "Tier: %s — used %.1f%% of %d cents.\n",
			q.Data.AITier, q.Data.BudgetUsedPct, q.Data.MonthlyBudgetCents)
	case "/model":
		models, err := s.Client.Models(ctx)
		if err != nil {
			return false, err
		}
		if arg == "" {
			fmt.Fprintln(s.Stdout, "Available models (use /model <id> to switch):")
			for _, m := range models {
				marker := ""
				if m.Default {
					marker = " *"
				}
				fmt.Fprintf(s.Stdout, "  %s%s — %s [%s]\n", m.ID, marker, m.Name, m.Tier)
			}
		} else {
			fmt.Fprintf(s.Stdout, "Model override (next message): %s\n", arg)
			fmt.Fprintln(s.Stdout, "(per-conversation override is not yet wired server-side; ignored for now)")
		}
	case "/memory", "/tools":
		fmt.Fprintln(s.Stdout, "Not yet implemented in v1.")
	case "/logout":
		fmt.Fprintln(s.Stdout, "Run: rvs logout")
	default:
		fmt.Fprintf(s.Stderr, "unknown command: %s (try /help)\n", cmd)
	}
	return false, nil
}

func (s *Session) printHelp() {
	fmt.Fprintln(s.Stdout, "Slash commands:")
	fmt.Fprintln(s.Stdout, "  /new [title]      start a new conversation")
	fmt.Fprintln(s.Stdout, "  /list             list recent conversations")
	fmt.Fprintln(s.Stdout, "  /open <id>        switch to conversation by id")
	fmt.Fprintln(s.Stdout, "  /rename <name>    rename current conversation")
	fmt.Fprintln(s.Stdout, "  /delete           delete current conversation")
	fmt.Fprintln(s.Stdout, "  /export md|json   export current conversation")
	fmt.Fprintln(s.Stdout, "  /model [id]       list models or pick one")
	fmt.Fprintln(s.Stdout, "  /tier             show tier and quota")
	fmt.Fprintln(s.Stdout, "  /memory           list memory facts (v1: not yet)")
	fmt.Fprintln(s.Stdout, "  /tools            list tools (v1: not yet)")
	fmt.Fprintln(s.Stdout, "  /clear            clear the screen")
	fmt.Fprintln(s.Stdout, "  /logout           erase the saved token")
	fmt.Fprintln(s.Stdout, "  /exit             leave")
}
