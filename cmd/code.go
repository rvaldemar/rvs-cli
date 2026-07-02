// Package cmd — `rvs code` runs an agentic coding loop against the
// rvs-openclaude sidecar via the Agents Hub. The sidecar holds the LLM and
// runs the QueryEngine; the laptop owns the file system and exposes
// Read/Write/Edit/Bash/Glob/Grep/WebFetch as external tools that the sidecar
// invokes via gRPC tool_execute callbacks.
//
// Wire path:
//
//	rvs code  ─WS─►  Hub /cable (CodeBridgeChannel) ─gRPC─►  rvs-openclaude
//	                                                           QueryEngine
//	                                                           ↓
//	                                                           provider
//
// All traffic is proto3-JSON-encoded openclaude.ClientMessage / ServerMessage
// wrapped in ActionCable frames.
package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/google/uuid"
	"github.com/spf13/cobra"

	"github.com/rvaldemar/rvs-cli/internal/bridge"
	openclaudev1 "github.com/rvaldemar/rvs-cli/internal/openclaude/v1"
	"github.com/rvaldemar/rvs-cli/internal/tools"
)

var codeCmd = &cobra.Command{
	Use:   "code [prompt...]",
	Short: "Agentic coding loop routed through the Hub",
	Long: `Run an agentic coding session against the rvs-openclaude sidecar via
the Hub. Local file-system tools (Read/Write/Edit/Bash/Glob/Grep/WebFetch)
execute on this laptop; the LLM runs in the sidecar.

The prompt may be passed as positional args ("rvs code fix the bug in X")
or piped via stdin ("echo 'fix the bug' | rvs code"). Use --print to write
only the final assistant text and skip the streamed intermediate output
(useful in scripts).

Examples:
  rvs code fix the failing test in spec/models/foo_spec.rb
  rvs code --print "summarize the README"
  echo "rename Foo to Bar everywhere" | rvs code
`,
	RunE: runCode,
}

func init() {
	codeCmd.Flags().Bool("print", false, "print only the final assistant text")
	codeCmd.Flags().String("model", "", "model override (passed to the sidecar; provider-specific)")
	codeCmd.Flags().Int32("max-turns", 20, "maximum agentic iterations before the sidecar yields")
}

func runCode(cmd *cobra.Command, args []string) error {
	_, creds, err := authenticatedClient(cmd)
	if err != nil {
		return err
	}

	prompt, err := resolvePrompt(args)
	if err != nil {
		return err
	}

	printOnly, _ := cmd.Flags().GetBool("print")
	model, _ := cmd.Flags().GetString("model")
	maxTurns, _ := cmd.Flags().GetInt32("max-turns")

	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("getwd: %w", err)
	}

	registry := tools.NewRegistry()
	prompter := tools.NewPrompter()
	runner := tools.NewRunner(registry, prompter)

	defs, err := registry.Definitions()
	if err != nil {
		return err
	}
	toolDefs := make([]*openclaudev1.ToolDefinition, len(defs))
	for i, d := range defs {
		toolDefs[i] = &openclaudev1.ToolDefinition{
			Name:            d.Name,
			Description:     d.Description,
			InputSchemaJson: d.InputSchemaJSON,
		}
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	client := bridge.New(creds.APIBase, creds.Token)
	if err := client.Connect(ctx); err != nil {
		return fmt.Errorf("connect bridge: %w", err)
	}
	defer client.Close()

	sessionID := client.SessionID()
	if sessionID == "" {
		sessionID = uuid.New().String()
	}

	initial := &openclaudev1.ClientMessage{
		Payload: &openclaudev1.ClientMessage_Request{
			Request: &openclaudev1.ChatRequest{
				Message:              prompt,
				WorkingDirectory:     cwd,
				SessionId:            sessionID,
				Tools:                toolDefs,
				DisabledBuiltinTools: registry.DisabledBuiltinNames(),
				MaxTurns:             &maxTurns,
			},
		},
	}
	if model != "" {
		initial.GetRequest().Model = &model
	}

	if err := client.Send(initial); err != nil {
		return fmt.Errorf("send initial request: %w", err)
	}

	return runEventLoop(ctx, client, runner, printOnly)
}

func runEventLoop(ctx context.Context, client *bridge.Client, runner *tools.Runner, printOnly bool) error {
	var (
		finalText  strings.Builder
		exitErr    error
		cancelSent bool
		stop       = ctx.Done()
	)

	flushFinal := func() {
		if printOnly && finalText.Len() > 0 {
			fmt.Print(finalText.String())
			if !strings.HasSuffix(finalText.String(), "\n") {
				fmt.Println()
			}
		}
	}

	for {
		select {
		case <-stop:
			if !cancelSent {
				_ = client.Send(&openclaudev1.ClientMessage{
					Payload: &openclaudev1.ClientMessage_Cancel{
						Cancel: &openclaudev1.CancelSignal{Reason: "user interrupt"},
					},
				})
				cancelSent = true
				stop = nil
				continue
			}
			flushFinal()
			return errors.New("interrupted")

		case ev, ok := <-client.Events():
			if !ok {
				flushFinal()
				return exitErr
			}
			switch ev.Kind {
			case bridge.EventReady:
				// Already consumed inside Connect; ignore late ones.
			case bridge.EventError:
				e := ev.Error
				flushFinal()
				if e == nil {
					return errors.New("bridge error")
				}
				return formatBridgeError(e)
			case bridge.EventPayload:
				if err := handleServerMessage(ctx, client, runner, ev.Payload, &finalText, printOnly); err != nil {
					return err
				}
				if isTerminalEvent(ev.Payload) {
					flushFinal()
					return nil
				}
			}
		}
	}
}

func handleServerMessage(
	ctx context.Context,
	client *bridge.Client,
	runner *tools.Runner,
	msg *openclaudev1.ServerMessage,
	finalText *strings.Builder,
	printOnly bool,
) error {
	switch ev := msg.Event.(type) {

	case *openclaudev1.ServerMessage_TextChunk:
		text := ev.TextChunk.GetText()
		if printOnly {
			finalText.WriteString(text)
			return nil
		}
		fmt.Print(text)

	case *openclaudev1.ServerMessage_ToolStart:
		if !printOnly {
			fmt.Fprintf(os.Stderr, "\n[→ %s %s]\n", ev.ToolStart.GetToolName(), summarize(ev.ToolStart.GetArgumentsJson()))
		}

	case *openclaudev1.ServerMessage_ToolResult:
		// Sidecar's text stream usually narrates the result already; we don't
		// re-echo it. Errors are surfaced by the LLM in its next turn.

	case *openclaudev1.ServerMessage_ActionRequired:
		// Built-in tool permission prompt from the sidecar (e.g. for sidecar-
		// owned tools we did not delegate). Simple yes/no via TTY.
		question := ev.ActionRequired.GetQuestion()
		fmt.Fprintf(os.Stderr, "\n[sidecar permission] %s\n[y/N]: ", question)
		approved := readYesNo()
		reply := "no"
		if approved {
			reply = "yes"
		}
		if err := client.Send(&openclaudev1.ClientMessage{
			Payload: &openclaudev1.ClientMessage_Input{
				Input: &openclaudev1.UserInput{
					PromptId: ev.ActionRequired.GetPromptId(),
					Reply:    reply,
				},
			},
		}); err != nil {
			return fmt.Errorf("send user input: %w", err)
		}

	case *openclaudev1.ServerMessage_ToolExecute:
		req := ev.ToolExecute
		if !printOnly {
			fmt.Fprintf(os.Stderr, "\n[execute %s]\n", req.GetToolName())
		}
		result := runner.Execute(ctx, req.GetToolName(), json.RawMessage(req.GetArgumentsJson()))
		resultJSON, err := json.Marshal(map[string]string{"output": result.Output})
		if err != nil {
			return fmt.Errorf("marshal tool result: %w", err)
		}
		if err := client.Send(&openclaudev1.ClientMessage{
			Payload: &openclaudev1.ClientMessage_ToolResponse{
				ToolResponse: &openclaudev1.ToolExecuteResponse{
					RequestId:  req.GetRequestId(),
					ResultJson: string(resultJSON),
					IsError:    result.IsError,
				},
			},
		}); err != nil {
			return fmt.Errorf("send tool response: %w", err)
		}

	case *openclaudev1.ServerMessage_Done:
		if !printOnly {
			fmt.Println()
			fmt.Fprintf(os.Stderr, "\n[done — %d in / %d out tokens]\n",
				ev.Done.GetPromptTokens(), ev.Done.GetCompletionTokens())
		}
		if printOnly && finalText.Len() == 0 {
			finalText.WriteString(ev.Done.GetFullText())
		}

	case *openclaudev1.ServerMessage_Error:
		fmt.Fprintf(os.Stderr, "\n[sidecar error %s] %s\n",
			ev.Error.GetCode(), ev.Error.GetMessage())
		return fmt.Errorf("sidecar error: %s", ev.Error.GetMessage())
	}
	return nil
}

func isTerminalEvent(msg *openclaudev1.ServerMessage) bool {
	if msg == nil {
		return false
	}
	switch msg.Event.(type) {
	case *openclaudev1.ServerMessage_Done, *openclaudev1.ServerMessage_Error:
		return true
	}
	return false
}

func resolvePrompt(args []string) (string, error) {
	if len(args) > 0 {
		return strings.Join(args, " "), nil
	}
	stat, err := os.Stdin.Stat()
	if err != nil {
		return "", fmt.Errorf("stat stdin: %w", err)
	}
	// If stdin is a terminal and no args were given, ask up front.
	if stat.Mode()&os.ModeCharDevice != 0 {
		return "", errors.New("provide a prompt as args or via stdin (e.g. `rvs code fix the bug` or `echo prompt | rvs code`)")
	}
	buf := make([]byte, 0, 4096)
	chunk := make([]byte, 4096)
	for {
		n, err := os.Stdin.Read(chunk)
		if n > 0 {
			buf = append(buf, chunk[:n]...)
		}
		if err != nil {
			break
		}
	}
	prompt := strings.TrimSpace(string(buf))
	if prompt == "" {
		return "", errors.New("empty prompt on stdin")
	}
	return prompt, nil
}

func summarize(rawArgs string) string {
	if rawArgs == "" {
		return ""
	}
	if len(rawArgs) > 200 {
		return rawArgs[:200] + "…"
	}
	return rawArgs
}

func formatBridgeError(e *bridge.ErrorData) error {
	if e.Type == "quota_exceeded" {
		return fmt.Errorf("quota exceeded: %s", e.Message)
	}
	return fmt.Errorf("%s: %s", e.Type, e.Message)
}

func readYesNo() bool {
	tty, err := os.OpenFile("/dev/tty", os.O_RDWR, 0)
	if err != nil {
		// No TTY → safest default is no.
		return false
	}
	defer tty.Close()
	buf := make([]byte, 16)
	n, _ := tty.Read(buf)
	if n == 0 {
		return false
	}
	r := strings.ToLower(strings.TrimSpace(string(buf[:n])))
	return r == "y" || r == "yes"
}
