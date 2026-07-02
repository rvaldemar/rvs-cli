package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/rvaldemar/rvs-cli/internal/api"
	"github.com/rvaldemar/rvs-cli/internal/config"
	"github.com/spf13/cobra"
)

const taskOutputLimit = 20000

var taskCmd = &cobra.Command{
	Use:   "task",
	Short: "Run Hub-issued agent tasks locally",
	Long: `Run Hub-issued agent tasks locally.

The Hub owns task contracts, leases and artifacts. This CLI owns local
execution. A task run executes only the commands declared on the claimed
AgentTask and reports the structured artifact back to the Hub.`,
}

var taskListCmd = &cobra.Command{
	Use:   "list",
	Short: "List recent agent tasks",
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := context.Background()
		client, _, err := taskClient(cmd)
		if err != nil {
			return err
		}
		tasks, err := client.ListAgentTasks(ctx)
		if err != nil {
			return err
		}
		if taskJSONFlag(cmd) {
			return printJSON(tasks)
		}
		if len(tasks) == 0 {
			fmt.Println("No agent tasks yet.")
			return nil
		}
		w := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
		fmt.Fprintln(w, "ID\tSTATUS\tPRIORITY\tUPDATED\tTITLE")
		for _, t := range tasks {
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n", t.ID, t.Status, t.Priority, t.UpdatedAt, t.Title)
		}
		return w.Flush()
	},
}

var taskShowCmd = &cobra.Command{
	Use:   "show TASK_ID",
	Short: "Show one agent task",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := context.Background()
		client, _, err := taskClient(cmd)
		if err != nil {
			return err
		}
		task, err := client.GetAgentTask(ctx, args[0])
		if err != nil {
			return err
		}
		if taskJSONFlag(cmd) {
			return printJSON(task)
		}
		printTask(task)
		return nil
	},
}

var taskCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "Create an agent task contract",
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := context.Background()
		client, _, err := taskClient(cmd)
		if err != nil {
			return err
		}
		title, _ := cmd.Flags().GetString("title")
		objective, _ := cmd.Flags().GetString("objective")
		if title == "" {
			return errors.New("--title is required")
		}
		if objective == "" {
			return errors.New("--objective is required")
		}
		commands, err := taskCommandsFromFlags(cmd)
		if err != nil {
			return err
		}
		acceptance, _ := cmd.Flags().GetStringArray("acceptance")
		repoPath, _ := cmd.Flags().GetString("repo")
		baseBranch, _ := cmd.Flags().GetString("base")
		branchName, _ := cmd.Flags().GetString("branch")
		ownerAgent, _ := cmd.Flags().GetString("owner-agent")
		priority, _ := cmd.Flags().GetString("priority")
		modelLane, _ := cmd.Flags().GetString("model-lane")

		task, err := client.CreateAgentTask(ctx, api.AgentTaskCreate{
			Title:      title,
			Objective:  objective,
			Priority:   priority,
			RepoPath:   repoPath,
			BaseBranch: baseBranch,
			BranchName: branchName,
			OwnerAgent: ownerAgent,
			ModelLane:  modelLane,
			Acceptance: acceptance,
			Commands:   commands,
			Constraints: map[string]any{
				"no_deploy":        true,
				"no_secret_reads":  true,
				"local_first_only": true,
			},
		})
		if err != nil {
			return err
		}
		if taskJSONFlag(cmd) {
			return printJSON(task)
		}
		fmt.Printf("Created %s (%s)\n", task.ID, task.Title)
		return nil
	},
}

var taskClaimCmd = &cobra.Command{
	Use:   "claim",
	Short: "Claim the next agent task",
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := context.Background()
		client, creds, err := taskClient(cmd)
		if err != nil {
			return err
		}
		claim := claimInput(cmd, creds)
		task, err := client.ClaimAgentTask(ctx, claim)
		if err != nil {
			return err
		}
		if task == nil {
			if taskJSONFlag(cmd) {
				return printJSON(map[string]any{"data": nil})
			}
			fmt.Println("No agent task available.")
			return nil
		}
		if taskJSONFlag(cmd) {
			return printJSON(task)
		}
		printTask(task)
		return nil
	},
}

var taskRunCmd = &cobra.Command{
	Use:   "run [TASK_ID]",
	Short: "Claim and execute an agent task's declared commands",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := context.Background()
		client, creds, err := taskClient(cmd)
		if err != nil {
			return err
		}
		claim := claimInput(cmd, creds)
		var task *api.AgentTask
		if len(args) == 1 {
			task, err = client.ClaimAgentTaskByID(ctx, args[0], claim)
		} else {
			task, err = client.ClaimAgentTask(ctx, claim)
		}
		if err != nil {
			return err
		}
		if task == nil {
			fmt.Println("No agent task available.")
			return nil
		}

		noSubmit, _ := cmd.Flags().GetBool("no-submit")
		timeoutSeconds, _ := cmd.Flags().GetInt("timeout")
		artifact, runErr := executeAgentTask(ctx, client, *task, claim.RunnerID, claim.LeaseSeconds, timeoutSeconds)
		summary := taskRunSummary(artifact)
		if noSubmit {
			if taskJSONFlag(cmd) {
				return printJSON(artifact)
			}
			fmt.Println(summary)
			return runErr
		}
		if runErr != nil {
			_, submitErr := client.FailAgentTask(ctx, task.ID, claim.RunnerID, runErr.Error(), false, artifact)
			if submitErr != nil {
				return fmt.Errorf("%w; additionally failed to report task failure: %v", runErr, submitErr)
			}
			return runErr
		}
		submitted, err := client.SubmitAgentTask(ctx, task.ID, claim.RunnerID, summary, artifact)
		if err != nil {
			return err
		}
		if taskJSONFlag(cmd) {
			return printJSON(submitted)
		}
		fmt.Printf("Submitted %s: %s\n", task.ID, summary)
		return nil
	},
}

var taskSubmitCmd = &cobra.Command{
	Use:   "submit TASK_ID",
	Short: "Submit a manual artifact for an agent task",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := context.Background()
		client, creds, err := taskClient(cmd)
		if err != nil {
			return err
		}
		artifact, err := artifactFromFlag(cmd)
		if err != nil {
			return err
		}
		summary, _ := cmd.Flags().GetString("summary")
		task, err := client.SubmitAgentTask(ctx, args[0], runnerIDFlag(cmd, creds), summary, artifact)
		if err != nil {
			return err
		}
		if taskJSONFlag(cmd) {
			return printJSON(task)
		}
		fmt.Printf("Submitted %s: %s\n", task.ID, task.Status)
		return nil
	},
}

var taskFailCmd = &cobra.Command{
	Use:   "fail TASK_ID",
	Short: "Mark an agent task failed or blocked",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := context.Background()
		client, creds, err := taskClient(cmd)
		if err != nil {
			return err
		}
		reason, _ := cmd.Flags().GetString("reason")
		if reason == "" {
			return errors.New("--reason is required")
		}
		blocked, _ := cmd.Flags().GetBool("blocked")
		artifact, err := artifactFromFlag(cmd)
		if err != nil {
			return err
		}
		task, err := client.FailAgentTask(ctx, args[0], runnerIDFlag(cmd, creds), reason, blocked, artifact)
		if err != nil {
			return err
		}
		if taskJSONFlag(cmd) {
			return printJSON(task)
		}
		fmt.Printf("%s %s: %s\n", strings.Title(task.Status), task.ID, reason)
		return nil
	},
}

var taskCancelCmd = &cobra.Command{
	Use:   "cancel TASK_ID",
	Short: "Cancel an agent task",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := context.Background()
		client, _, err := taskClient(cmd)
		if err != nil {
			return err
		}
		task, err := client.CancelAgentTask(ctx, args[0])
		if err != nil {
			return err
		}
		if taskJSONFlag(cmd) {
			return printJSON(task)
		}
		fmt.Printf("Canceled %s\n", task.ID)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(taskCmd)

	taskCmd.AddCommand(taskListCmd, taskShowCmd, taskCreateCmd, taskClaimCmd, taskRunCmd, taskSubmitCmd, taskFailCmd, taskCancelCmd)
	for _, c := range []*cobra.Command{taskListCmd, taskShowCmd, taskCreateCmd, taskClaimCmd, taskRunCmd, taskSubmitCmd, taskFailCmd, taskCancelCmd} {
		c.Flags().Bool("json", false, "write JSON output")
	}
	taskCreateCmd.Flags().String("title", "", "task title")
	taskCreateCmd.Flags().String("objective", "", "task objective")
	taskCreateCmd.Flags().String("repo", "", "repository path for local execution")
	taskCreateCmd.Flags().String("base", "main", "base branch")
	taskCreateCmd.Flags().String("branch", "", "working branch name")
	taskCreateCmd.Flags().String("owner-agent", "", "owner agent label")
	taskCreateCmd.Flags().String("priority", "normal", "priority: low, normal, high, urgent")
	taskCreateCmd.Flags().String("model-lane", "T2", "model capability lane")
	taskCreateCmd.Flags().StringArray("acceptance", nil, "acceptance criterion; repeatable")
	taskCreateCmd.Flags().StringArray("cmd", nil, "command to run; repeatable")

	for _, c := range []*cobra.Command{taskClaimCmd, taskRunCmd, taskSubmitCmd, taskFailCmd} {
		c.Flags().String("runner", "", "runner id (default: hostname/user)")
	}
	for _, c := range []*cobra.Command{taskClaimCmd, taskRunCmd} {
		c.Flags().String("repo", "", "claim only tasks for this repo path")
		c.Flags().String("owner-agent", "", "claim only tasks for this owner agent")
		c.Flags().Int("lease", 900, "lease seconds")
	}
	taskRunCmd.Flags().Bool("no-submit", false, "execute locally without reporting back to the Hub")
	taskRunCmd.Flags().Int("timeout", 900, "timeout seconds per command")
	taskSubmitCmd.Flags().String("summary", "", "submission summary")
	taskSubmitCmd.Flags().String("artifact", "", "JSON artifact file")
	taskFailCmd.Flags().String("reason", "", "failure reason")
	taskFailCmd.Flags().Bool("blocked", false, "mark as blocked instead of failed")
	taskFailCmd.Flags().String("artifact", "", "JSON artifact file")
}

func taskClient(cmd *cobra.Command) (*api.Client, config.Credentials, error) {
	return authenticatedClient(cmd)
}

func taskJSONFlag(cmd *cobra.Command) bool {
	value, _ := cmd.Flags().GetBool("json")
	return value
}

func runnerIDFlag(cmd *cobra.Command, creds config.Credentials) string {
	runner, _ := cmd.Flags().GetString("runner")
	if runner != "" {
		return runner
	}
	if env := strings.TrimSpace(os.Getenv("RVS_RUNNER_ID")); env != "" {
		return env
	}
	host, _ := os.Hostname()
	if host == "" {
		host = "local"
	}
	if creds.UserEmail != "" {
		return host + "/" + creds.UserEmail
	}
	return host + "/rvs-cli"
}

func claimInput(cmd *cobra.Command, creds config.Credentials) api.AgentTaskClaim {
	repoPath, _ := cmd.Flags().GetString("repo")
	ownerAgent, _ := cmd.Flags().GetString("owner-agent")
	lease, _ := cmd.Flags().GetInt("lease")
	return api.AgentTaskClaim{
		RunnerID:     runnerIDFlag(cmd, creds),
		RepoPath:     repoPath,
		OwnerAgent:   ownerAgent,
		LeaseSeconds: lease,
	}
}

func taskCommandsFromFlags(cmd *cobra.Command) ([]api.AgentTaskCommand, error) {
	raw, _ := cmd.Flags().GetStringArray("cmd")
	out := make([]api.AgentTaskCommand, 0, len(raw))
	for i, run := range raw {
		run = strings.TrimSpace(run)
		if run == "" {
			return nil, errors.New("--cmd cannot be empty")
		}
		out = append(out, api.AgentTaskCommand{Name: fmt.Sprintf("cmd-%d", i+1), Run: run})
	}
	return out, nil
}

func artifactFromFlag(cmd *cobra.Command) (map[string]any, error) {
	path, _ := cmd.Flags().GetString("artifact")
	if path == "" {
		return map[string]any{}, nil
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var artifact map[string]any
	if err := json.Unmarshal(raw, &artifact); err != nil {
		return nil, fmt.Errorf("parse artifact JSON: %w", err)
	}
	return artifact, nil
}

func executeAgentTask(ctx context.Context, client *api.Client, task api.AgentTask, runnerID string, leaseSeconds, timeoutSeconds int) (map[string]any, error) {
	repoPath := task.RepoPath
	if repoPath == "" {
		cwd, err := os.Getwd()
		if err != nil {
			return nil, err
		}
		repoPath = cwd
	}
	absRepo, err := filepath.Abs(repoPath)
	if err != nil {
		return nil, err
	}
	if info, err := os.Stat(absRepo); err != nil {
		return nil, fmt.Errorf("repo path: %w", err)
	} else if !info.IsDir() {
		return nil, fmt.Errorf("repo path is not a directory: %s", absRepo)
	}

	started := time.Now().UTC()
	results := make([]map[string]any, 0, len(task.Commands))
	artifact := map[string]any{
		"runner_id":  runnerID,
		"repo_path":  absRepo,
		"started_at": started.Format(time.RFC3339),
		"commands":   results,
	}
	if len(task.Commands) == 0 {
		artifact["finished_at"] = time.Now().UTC().Format(time.RFC3339)
		artifact["status"] = "no_commands"
		return artifact, errors.New("task has no commands to run")
	}

	for _, command := range task.Commands {
		if _, err := client.HeartbeatAgentTask(ctx, task.ID, runnerID, leaseSeconds); err != nil {
			return artifact, fmt.Errorf("heartbeat before command: %w", err)
		}
		result := runShellCommand(ctx, absRepo, command, timeoutSeconds)
		results = append(results, result)
		artifact["commands"] = results
		if _, err := client.HeartbeatAgentTask(ctx, task.ID, runnerID, leaseSeconds); err != nil {
			return artifact, fmt.Errorf("heartbeat after command: %w", err)
		}
		if code, _ := result["exit_code"].(int); code != 0 {
			artifact["finished_at"] = time.Now().UTC().Format(time.RFC3339)
			artifact["status"] = "failed"
			return artifact, fmt.Errorf("command failed: %s", commandName(command))
		}
	}
	artifact["finished_at"] = time.Now().UTC().Format(time.RFC3339)
	artifact["status"] = "passed"
	return artifact, nil
}

func runShellCommand(parent context.Context, dir string, command api.AgentTaskCommand, timeoutSeconds int) map[string]any {
	timeout := time.Duration(timeoutSeconds) * time.Second
	if timeout <= 0 {
		timeout = 15 * time.Minute
	}
	ctx, cancel := context.WithTimeout(parent, timeout)
	defer cancel()

	started := time.Now().UTC()
	cmd := exec.CommandContext(ctx, "sh", "-lc", command.Run)
	cmd.Dir = dir
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	finished := time.Now().UTC()

	exitCode := 0
	if err != nil {
		exitCode = 1
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			exitCode = exitErr.ExitCode()
		}
		if ctx.Err() == context.DeadlineExceeded {
			exitCode = 124
		}
	}

	return map[string]any{
		"name":        commandName(command),
		"run":         command.Run,
		"exit_code":   exitCode,
		"started_at":  started.Format(time.RFC3339),
		"finished_at": finished.Format(time.RFC3339),
		"duration_ms": finished.Sub(started).Milliseconds(),
		"stdout":      truncateString(stdout.String(), taskOutputLimit),
		"stderr":      truncateString(stderr.String(), taskOutputLimit),
	}
}

func taskRunSummary(artifact map[string]any) string {
	status, _ := artifact["status"].(string)
	if status == "" {
		status = "unknown"
	}
	commands, _ := artifact["commands"].([]map[string]any)
	return fmt.Sprintf("%s (%d commands)", status, len(commands))
}

func commandName(command api.AgentTaskCommand) string {
	if command.Name != "" {
		return command.Name
	}
	return command.Run
}

func truncateString(value string, limit int) string {
	if len(value) <= limit {
		return value
	}
	return value[:limit] + "\n[truncated]"
}

func printTask(task *api.AgentTask) {
	if task == nil {
		fmt.Println("No agent task.")
		return
	}
	fmt.Printf("ID:        %s\n", task.ID)
	fmt.Printf("Status:    %s\n", task.Status)
	fmt.Printf("Priority:  %s\n", task.Priority)
	fmt.Printf("Title:     %s\n", task.Title)
	fmt.Printf("Objective: %s\n", task.Objective)
	if task.RepoPath != "" {
		fmt.Printf("Repo:      %s\n", task.RepoPath)
	}
	if task.ClaimedBy != "" {
		fmt.Printf("Runner:    %s\n", task.ClaimedBy)
	}
	if len(task.Commands) > 0 {
		fmt.Println("Commands:")
		for _, c := range task.Commands {
			fmt.Printf("  - %s\n", c.Run)
		}
	}
}

func printJSON(value any) error {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(value)
}
