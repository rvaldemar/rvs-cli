// Package api wraps the Agents Hub HTTP API used by the CLI.
//
// All requests carry the bearer token from credentials. Streaming endpoints
// (POST messages) return a *MessageStream that emits SSE events until the
// `done` event arrives.
package api

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const userAgent = "rvs-cli"

type Client struct {
	baseURL string
	token   string
	http    *http.Client
}

func New(baseURL, token string) *Client {
	return &Client{
		baseURL: strings.TrimRight(baseURL, "/"),
		token:   token,
		http: &http.Client{
			Timeout: 60 * time.Second,
		},
	}
}

type APIError struct {
	Status int
	Body   string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("api: %d %s", e.Status, e.Body)
}

// QuotaError is returned when the server replies 402 quota_exceeded.
type QuotaError struct {
	Scope   string `json:"scope"`
	ResetAt string `json:"reset_at"`
	Usage   int64  `json:"usage"`
	Cap     int64  `json:"cap"`
	Message string `json:"message"`
}

func (e *QuotaError) Error() string {
	return fmt.Sprintf("quota exceeded (%s, resets %s): %d/%d", e.Scope, e.ResetAt, e.Usage, e.Cap)
}

func (e *QuotaError) UnmarshalJSON(data []byte) error {
	var raw struct {
		Scope   string          `json:"scope"`
		ResetAt string          `json:"reset_at"`
		Usage   json.RawMessage `json:"usage"`
		Cap     json.RawMessage `json:"cap"`
		Message string          `json:"message"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	e.Scope = raw.Scope
	e.ResetAt = raw.ResetAt
	e.Message = raw.Message

	var err error
	if len(raw.Usage) > 0 {
		e.Usage, err = quotaAmount(raw.Usage)
		if err != nil {
			return fmt.Errorf("quota usage: %w", err)
		}
	}
	if len(raw.Cap) > 0 {
		e.Cap, err = quotaAmount(raw.Cap)
		if err != nil {
			return fmt.Errorf("quota cap: %w", err)
		}
	}
	return nil
}

func quotaAmount(data []byte) (int64, error) {
	var scalar int64
	if err := json.Unmarshal(data, &scalar); err == nil {
		return scalar, nil
	}

	var obj struct {
		Tokens    int64 `json:"tokens"`
		CostCents int64 `json:"cost_cents"`
	}
	if err := json.Unmarshal(data, &obj); err != nil {
		return 0, err
	}
	if obj.Tokens != 0 {
		return obj.Tokens, nil
	}
	return obj.CostCents, nil
}

func (c *Client) do(ctx context.Context, method, path string, body any) (*http.Response, error) {
	var rdr io.Reader
	if body != nil {
		buf, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		rdr = bytes.NewReader(buf)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, rdr)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	return c.http.Do(req)
}

func (c *Client) decode(resp *http.Response, out any) error {
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		if resp.StatusCode == http.StatusPaymentRequired {
			var qe QuotaError
			if err := json.Unmarshal(body, &qe); err == nil && qe.Scope != "" {
				return &qe
			}
		}
		return &APIError{Status: resp.StatusCode, Body: string(body)}
	}
	if out == nil {
		return nil
	}
	return json.Unmarshal(body, out)
}

// === Auth ===

type loginUser struct {
	Email string `json:"email"`
	Name  string `json:"name"`
}

type loginOrg struct {
	Slug string `json:"slug"`
	Name string `json:"name"`
}

type LoginResp struct {
	Token  string    `json:"token"`
	Prefix string    `json:"token_prefix"`
	Label  string    `json:"label"`
	User   loginUser `json:"user"`
	Org    loginOrg  `json:"organization"`
}

func (c *Client) Login(ctx context.Context, email, password, label string) (*LoginResp, error) {
	resp, err := c.do(ctx, "POST", "/api/v1/auth/cli/login", map[string]string{
		"email":    email,
		"password": password,
		"label":    label,
	})
	if err != nil {
		return nil, err
	}
	out := &LoginResp{}
	if err := c.decode(resp, out); err != nil {
		return nil, err
	}
	return out, nil
}

// === Identity ===

type MeUser struct {
	ID    ID     `json:"id"`
	Email string `json:"email"`
	Name  string `json:"name"`
}

type MeOrg struct {
	ID             ID      `json:"id"`
	Name           string  `json:"name"`
	AITier         string  `json:"ai_tier"`
	TierConfigName string  `json:"tier_config_name"`
	Runtime        Runtime `json:"runtime"`
}

type ID string

func (id *ID) UnmarshalJSON(data []byte) error {
	if string(data) == "null" {
		*id = ""
		return nil
	}
	var s string
	if err := json.Unmarshal(data, &s); err == nil {
		*id = ID(s)
		return nil
	}
	*id = ID(string(data))
	return nil
}

type Runtime struct {
	Provider   string `json:"provider"`
	Model      string `json:"model"`
	Status     string `json:"status"`
	Configured bool   `json:"configured"`
}

type Me struct {
	Data struct {
		User         *MeUser `json:"user"`
		Organization MeOrg   `json:"organization"`
	} `json:"data"`
}

func (c *Client) Me(ctx context.Context) (*Me, error) {
	resp, err := c.do(ctx, "GET", "/api/v1/me", nil)
	if err != nil {
		return nil, err
	}
	out := &Me{}
	if err := c.decode(resp, out); err != nil {
		return nil, err
	}
	return out, nil
}

type Quota struct {
	Data struct {
		AITier                string  `json:"ai_tier"`
		TierConfigName        string  `json:"tier_config_name"`
		QuotaScope            string  `json:"quota_scope"`
		MonthlyTokenCap       int64   `json:"monthly_token_cap"`
		MonthlyCostCapCents   int64   `json:"monthly_cost_cap_cents"`
		CurrentMonthTokens    int64   `json:"current_month_tokens"`
		CurrentMonthCostCents int64   `json:"current_month_cost_cents"`
		QuotaUsedPct          float64 `json:"quota_used_pct"`
		MonthlyBudgetCents    int64   `json:"monthly_budget_cents"`
		CurrentMonthSpend     int64   `json:"current_month_spend_cents"`
		BudgetUsedPct         float64 `json:"budget_used_pct"`
		EnforcementAction     string  `json:"enforcement_action"`
		OverBudget            bool    `json:"over_budget"`
		PeriodStart           string  `json:"period_start"`
		Runtime               Runtime `json:"runtime"`
	} `json:"data"`
}

func (c *Client) Quota(ctx context.Context) (*Quota, error) {
	resp, err := c.do(ctx, "GET", "/api/v1/quota", nil)
	if err != nil {
		return nil, err
	}
	out := &Quota{}
	if err := c.decode(resp, out); err != nil {
		return nil, err
	}
	return out, nil
}

// === Models ===

type ModelInfo struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Provider string `json:"provider"`
	Tier     string `json:"tier"`
	Default  bool   `json:"default"`
}

type modelsResp struct {
	Data    []ModelInfo `json:"data"`
	Runtime Runtime     `json:"runtime"`
}

func (c *Client) Models(ctx context.Context) ([]ModelInfo, error) {
	resp, err := c.do(ctx, "GET", "/api/v1/models", nil)
	if err != nil {
		return nil, err
	}
	out := &modelsResp{}
	if err := c.decode(resp, out); err != nil {
		return nil, err
	}
	return out.Data, nil
}

// === Agent tasks ===

type AgentTaskCommand struct {
	Name string `json:"name,omitempty"`
	Run  string `json:"run"`
}

type AgentTask struct {
	ID             string             `json:"id"`
	Title          string             `json:"title"`
	Objective      string             `json:"objective"`
	Status         string             `json:"status"`
	Priority       string             `json:"priority"`
	RepoPath       string             `json:"repo_path"`
	BaseBranch     string             `json:"base_branch"`
	BranchName     string             `json:"branch_name"`
	OwnerAgent     string             `json:"owner_agent"`
	ModelLane      string             `json:"model_lane"`
	Scope          map[string]any     `json:"scope"`
	Acceptance     []string           `json:"acceptance"`
	Commands       []AgentTaskCommand `json:"commands"`
	Constraints    map[string]any     `json:"constraints"`
	Artifact       map[string]any     `json:"artifact"`
	Metadata       map[string]any     `json:"metadata"`
	ClaimedBy      string             `json:"claimed_by"`
	ClaimedAt      string             `json:"claimed_at"`
	HeartbeatAt    string             `json:"heartbeat_at"`
	LeaseExpiresAt string             `json:"lease_expires_at"`
	SubmittedAt    string             `json:"submitted_at"`
	FailureReason  string             `json:"failure_reason"`
	CreatedAt      string             `json:"created_at"`
	UpdatedAt      string             `json:"updated_at"`
}

type AgentTaskCreate struct {
	Title       string             `json:"title"`
	Objective   string             `json:"objective"`
	Priority    string             `json:"priority,omitempty"`
	RepoPath    string             `json:"repo_path,omitempty"`
	BaseBranch  string             `json:"base_branch,omitempty"`
	BranchName  string             `json:"branch_name,omitempty"`
	OwnerAgent  string             `json:"owner_agent,omitempty"`
	ModelLane   string             `json:"model_lane,omitempty"`
	Scope       map[string]any     `json:"scope,omitempty"`
	Acceptance  []string           `json:"acceptance,omitempty"`
	Commands    []AgentTaskCommand `json:"commands,omitempty"`
	Constraints map[string]any     `json:"constraints,omitempty"`
	Metadata    map[string]any     `json:"metadata,omitempty"`
}

type AgentTaskClaim struct {
	RunnerID     string `json:"runner_id,omitempty"`
	RepoPath     string `json:"repo_path,omitempty"`
	OwnerAgent   string `json:"owner_agent,omitempty"`
	LeaseSeconds int    `json:"lease_seconds,omitempty"`
}

type agentTasksResp struct {
	Data []AgentTask `json:"data"`
}

type agentTaskResp struct {
	Data *AgentTask `json:"data"`
}

func (c *Client) ListAgentTasks(ctx context.Context) ([]AgentTask, error) {
	resp, err := c.do(ctx, "GET", "/api/v1/agent_tasks", nil)
	if err != nil {
		return nil, err
	}
	out := &agentTasksResp{}
	if err := c.decode(resp, out); err != nil {
		return nil, err
	}
	return out.Data, nil
}

func (c *Client) GetAgentTask(ctx context.Context, id string) (*AgentTask, error) {
	resp, err := c.do(ctx, "GET", "/api/v1/agent_tasks/"+id, nil)
	if err != nil {
		return nil, err
	}
	out := &agentTaskResp{}
	if err := c.decode(resp, out); err != nil {
		return nil, err
	}
	return out.Data, nil
}

func (c *Client) CreateAgentTask(ctx context.Context, input AgentTaskCreate) (*AgentTask, error) {
	resp, err := c.do(ctx, "POST", "/api/v1/agent_tasks", map[string]any{
		"agent_task": input,
	})
	if err != nil {
		return nil, err
	}
	out := &agentTaskResp{}
	if err := c.decode(resp, out); err != nil {
		return nil, err
	}
	return out.Data, nil
}

func (c *Client) ClaimAgentTask(ctx context.Context, input AgentTaskClaim) (*AgentTask, error) {
	resp, err := c.do(ctx, "POST", "/api/v1/agent_tasks/claim", input)
	if err != nil {
		return nil, err
	}
	out := &agentTaskResp{}
	if err := c.decode(resp, out); err != nil {
		return nil, err
	}
	return out.Data, nil
}

func (c *Client) ClaimAgentTaskByID(ctx context.Context, id string, input AgentTaskClaim) (*AgentTask, error) {
	resp, err := c.do(ctx, "POST", "/api/v1/agent_tasks/"+id+"/claim", input)
	if err != nil {
		return nil, err
	}
	out := &agentTaskResp{}
	if err := c.decode(resp, out); err != nil {
		return nil, err
	}
	return out.Data, nil
}

func (c *Client) HeartbeatAgentTask(ctx context.Context, id, runnerID string, leaseSeconds int) (*AgentTask, error) {
	resp, err := c.do(ctx, "POST", "/api/v1/agent_tasks/"+id+"/heartbeat", map[string]any{
		"runner_id":     runnerID,
		"lease_seconds": leaseSeconds,
	})
	if err != nil {
		return nil, err
	}
	out := &agentTaskResp{}
	if err := c.decode(resp, out); err != nil {
		return nil, err
	}
	return out.Data, nil
}

func (c *Client) SubmitAgentTask(ctx context.Context, id, runnerID, summary string, artifact map[string]any) (*AgentTask, error) {
	resp, err := c.do(ctx, "POST", "/api/v1/agent_tasks/"+id+"/submit", map[string]any{
		"runner_id": runnerID,
		"summary":   summary,
		"artifact":  artifact,
	})
	if err != nil {
		return nil, err
	}
	out := &agentTaskResp{}
	if err := c.decode(resp, out); err != nil {
		return nil, err
	}
	return out.Data, nil
}

func (c *Client) FailAgentTask(ctx context.Context, id, runnerID, reason string, blocked bool, artifact map[string]any) (*AgentTask, error) {
	resp, err := c.do(ctx, "POST", "/api/v1/agent_tasks/"+id+"/fail", map[string]any{
		"runner_id": runnerID,
		"reason":    reason,
		"blocked":   blocked,
		"artifact":  artifact,
	})
	if err != nil {
		return nil, err
	}
	out := &agentTaskResp{}
	if err := c.decode(resp, out); err != nil {
		return nil, err
	}
	return out.Data, nil
}

func (c *Client) CancelAgentTask(ctx context.Context, id string) (*AgentTask, error) {
	resp, err := c.do(ctx, "POST", "/api/v1/agent_tasks/"+id+"/cancel", nil)
	if err != nil {
		return nil, err
	}
	out := &agentTaskResp{}
	if err := c.decode(resp, out); err != nil {
		return nil, err
	}
	return out.Data, nil
}

// === Effort entries ===

type EffortEntry struct {
	ID        string `json:"id"`
	Minutes   int    `json:"minutes"`
	Category  string `json:"category"`
	Note      string `json:"note"`
	CreatedAt string `json:"created_at"`
}

func (c *Client) LogEffortEntry(ctx context.Context, minutes int, category, note string) (*EffortEntry, error) {
	body := map[string]any{
		"effort_entry": map[string]any{
			"minutes":  minutes,
			"category": category,
			"note":     note,
		},
	}
	resp, err := c.do(ctx, "POST", "/api/v1/effort_entries", body)
	if err != nil {
		return nil, err
	}
	var out struct {
		Data EffortEntry `json:"data"`
	}
	if err := c.decode(resp, &out); err != nil {
		return nil, err
	}
	return &out.Data, nil
}

// === Playbook templates ===

type PlaybookTemplate struct {
	ID        string `json:"id"`
	Code      string `json:"code"`
	Domain    string `json:"domain"`
	Name      string `json:"name"`
	Version   string `json:"version"`
	Certified bool   `json:"certified"`
	CreatedAt string `json:"created_at"`
}

type PlaybookInstance struct {
	ID         string `json:"id"`
	Code       string `json:"code"`
	TemplateID string `json:"template_id"`
	CreatedAt  string `json:"created_at"`
}

func (c *Client) ListPlaybookTemplates(ctx context.Context, domain string) ([]PlaybookTemplate, error) {
	path := "/api/v1/playbook_templates"
	if domain != "" {
		path += "?domain=" + domain
	}
	resp, err := c.do(ctx, "GET", path, nil)
	if err != nil {
		return nil, err
	}
	var out struct {
		Data []PlaybookTemplate `json:"data"`
	}
	if err := c.decode(resp, &out); err != nil {
		return nil, err
	}
	return out.Data, nil
}

func (c *Client) InstantiatePlaybookTemplate(ctx context.Context, id, code string) (*PlaybookInstance, error) {
	body := map[string]any{}
	if code != "" {
		body["playbook"] = map[string]string{"code": code}
	}
	resp, err := c.do(ctx, "POST", "/api/v1/playbook_templates/"+id+"/instantiate", body)
	if err != nil {
		return nil, err
	}
	var out struct {
		Data PlaybookInstance `json:"data"`
	}
	if err := c.decode(resp, &out); err != nil {
		return nil, err
	}
	return &out.Data, nil
}

// === Conversations ===

type Conversation struct {
	ID        string `json:"id"`
	Title     string `json:"title"`
	UpdatedAt string `json:"updated_at"`
}

type convsResp struct {
	Data []Conversation `json:"data"`
}

func (c *Client) ListConversations(ctx context.Context) ([]Conversation, error) {
	resp, err := c.do(ctx, "GET", "/api/v1/conversations", nil)
	if err != nil {
		return nil, err
	}
	out := &convsResp{}
	if err := c.decode(resp, out); err != nil {
		return nil, err
	}
	return out.Data, nil
}

func (c *Client) CreateConversation(ctx context.Context, title string) (*Conversation, error) {
	resp, err := c.do(ctx, "POST", "/api/v1/conversations", map[string]any{
		"conversation": map[string]string{"title": title},
	})
	if err != nil {
		return nil, err
	}
	var out struct {
		Data Conversation `json:"data"`
	}
	if err := c.decode(resp, &out); err != nil {
		return nil, err
	}
	return &out.Data, nil
}

func (c *Client) RenameConversation(ctx context.Context, id, title string) error {
	resp, err := c.do(ctx, "PATCH", "/api/v1/conversations/"+id, map[string]any{
		"conversation": map[string]string{"title": title},
	})
	if err != nil {
		return err
	}
	return c.decode(resp, nil)
}

func (c *Client) DeleteConversation(ctx context.Context, id string) error {
	resp, err := c.do(ctx, "DELETE", "/api/v1/conversations/"+id, nil)
	if err != nil {
		return err
	}
	return c.decode(resp, nil)
}

func (c *Client) ExportConversation(ctx context.Context, id, format string) (string, error) {
	resp, err := c.do(ctx, "GET", "/api/v1/conversations/"+id+"/export?format="+format, nil)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		return "", &APIError{Status: resp.StatusCode, Body: string(body)}
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	return string(body), nil
}

// === SSE message streaming ===

type StreamEvent struct {
	Type string
	Data json.RawMessage
}

// StreamMessage POSTs the user message and yields SSE events to the handler
// until the `done` or `error` event arrives. Cancelling ctx aborts the stream.
func (c *Client) StreamMessage(ctx context.Context, conversationID, content string, handler func(StreamEvent) error) error {
	body, err := json.Marshal(map[string]string{"content": content})
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, "POST",
		c.baseURL+"/api/v1/conversations/"+conversationID+"/messages",
		bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")
	req.Header.Set("Authorization", "Bearer "+c.token)

	// No client-level timeout for streams; rely on ctx.
	stream := &http.Client{Timeout: 0}
	resp, err := stream.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		raw, _ := io.ReadAll(resp.Body)
		if resp.StatusCode == http.StatusPaymentRequired {
			var qe QuotaError
			if err := json.Unmarshal(raw, &qe); err == nil && qe.Scope != "" {
				return &qe
			}
		}
		return &APIError{Status: resp.StatusCode, Body: string(raw)}
	}

	reader := bufio.NewReader(resp.Body)
	var ev StreamEvent
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			if errors.Is(err, io.EOF) {
				return nil
			}
			return err
		}
		line = strings.TrimRight(line, "\r\n")
		switch {
		case strings.HasPrefix(line, "event: "):
			ev.Type = strings.TrimPrefix(line, "event: ")
		case strings.HasPrefix(line, "data: "):
			ev.Data = json.RawMessage(strings.TrimPrefix(line, "data: "))
		case line == "":
			if ev.Type != "" {
				if err := handler(ev); err != nil {
					return err
				}
				if ev.Type == "done" || ev.Type == "error" {
					return nil
				}
				ev = StreamEvent{}
			}
		}
	}
}
