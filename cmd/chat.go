package cmd

import (
	"context"
	"errors"

	"github.com/rvaldemar/rvs-openclaude/cli/internal/api"
	"github.com/rvaldemar/rvs-openclaude/cli/internal/chat"
	"github.com/rvaldemar/rvs-openclaude/cli/internal/config"
	"github.com/spf13/cobra"
)

var chatCmd = &cobra.Command{
	Use:   "chat [conversation-id]",
	Short: "Start an interactive chat session",
	Long: `Open the chat REPL.

If a conversation id is provided, resume it. Otherwise a new conversation
is created. Use /help once inside to see the slash commands.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		creds, err := config.Load()
		if err != nil {
			return err
		}
		if creds.Empty() {
			return errors.New("not logged in. Run: rvs login")
		}
		client := api.New(creds.APIBase, creds.Token)
		session := chat.New(client, creds.APIBase, creds.UserEmail)
		if len(args) == 1 {
			session.Conv = &api.Conversation{ID: args[0]}
		}
		return session.Run(ctx)
	},
}
