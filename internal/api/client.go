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
	ID    string `json:"id"`
	Email string `json:"email"`
	Name  string `json:"name"`
}

type MeOrg struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	AITier string `json:"ai_tier"`
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
		AITier              string  `json:"ai_tier"`
		MonthlyBudgetCents  int64   `json:"monthly_budget_cents"`
		CurrentMonthSpend   int64   `json:"current_month_spend_cents"`
		BudgetUsedPct       float64 `json:"budget_used_pct"`
		EnforcementAction   string  `json:"enforcement_action"`
		OverBudget          bool    `json:"over_budget"`
		PeriodStart         string  `json:"period_start"`
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
	Data []ModelInfo `json:"data"`
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
