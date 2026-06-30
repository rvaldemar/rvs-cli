package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func newServer(t *testing.T, handler http.HandlerFunc) (*httptest.Server, *Client) {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	return srv, New(srv.URL, "test-token")
}

func TestLoginSuccess(t *testing.T) {
	_, c := newServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/auth/cli/login" {
			t.Errorf("path: %s", r.URL.Path)
		}
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"token":        "abc",
			"token_prefix": "abcdef12",
			"label":        "test",
			"user":         map[string]string{"email": "a@b", "name": "Alice"},
			"organization": map[string]string{"slug": "acme", "name": "Acme"},
		})
	})

	resp, err := c.Login(context.Background(), "a@b", "p", "test")
	if err != nil {
		t.Fatalf("Login: %v", err)
	}
	if resp.Token != "abc" || resp.User.Email != "a@b" || resp.Org.Slug != "acme" {
		t.Errorf("unexpected: %+v", resp)
	}
}

func TestQuotaErrorOn402(t *testing.T) {
	_, c := newServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusPaymentRequired)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"scope":    "user",
			"reset_at": "2026-05-01T00:00:00Z",
			"usage":    100,
			"cap":      50,
			"message":  "user cap reached",
		})
	})

	_, err := c.ListConversations(context.Background())
	if err == nil {
		t.Fatal("expected error")
	}
	qe, ok := err.(*QuotaError)
	if !ok {
		t.Fatalf("want *QuotaError, got %T: %v", err, err)
	}
	if qe.Scope != "user" || qe.Cap != 50 {
		t.Errorf("unexpected: %+v", qe)
	}
}

func TestQuotaErrorOn402WithStructuredUsage(t *testing.T) {
	_, c := newServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusPaymentRequired)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"scope":    "user",
			"reset_at": "2026-05-01T00:00:00Z",
			"usage":    map[string]any{"tokens": 150, "cost_cents": 20},
			"cap":      map[string]any{"tokens": 100, "cost_cents": 50},
			"message":  "user cap reached",
		})
	})

	_, err := c.ListConversations(context.Background())
	if err == nil {
		t.Fatal("expected error")
	}
	qe, ok := err.(*QuotaError)
	if !ok {
		t.Fatalf("want *QuotaError, got %T: %v", err, err)
	}
	if qe.Usage != 150 || qe.Cap != 100 {
		t.Errorf("unexpected: %+v", qe)
	}
}

func TestMeIncludesRuntime(t *testing.T) {
	_, c := newServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/me" {
			t.Errorf("path: %s", r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": map[string]any{
				"user": map[string]any{"id": 1, "email": "a@b", "name": "Alice"},
				"organization": map[string]any{
					"id":               2,
					"name":             "Acme",
					"ai_tier":          "standard",
					"tier_config_name": "gpt_codex_subscription",
					"runtime": map[string]any{
						"provider":   "openai",
						"model":      "gpt-4.1-mini",
						"status":     "configured",
						"configured": true,
					},
				},
			},
		})
	})

	me, err := c.Me(context.Background())
	if err != nil {
		t.Fatalf("Me: %v", err)
	}
	if me.Data.Organization.Runtime.Provider != "openai" || me.Data.Organization.Runtime.Model != "gpt-4.1-mini" {
		t.Errorf("unexpected runtime: %+v", me.Data.Organization.Runtime)
	}
}

func TestStreamMessageEvents(t *testing.T) {
	body := strings.Join([]string{
		"event: start",
		`data: {"conversation_id":"c1"}`,
		"",
		"event: chunk",
		`data: {"delta":"Hello"}`,
		"",
		"event: chunk",
		`data: {"delta":" world"}`,
		"",
		"event: done",
		`data: {}`,
		"",
	}, "\n") + "\n"

	_, c := newServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/conversations/c1/messages" {
			t.Errorf("path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(body))
	})

	var got []string
	err := c.StreamMessage(context.Background(), "c1", "hi", func(ev StreamEvent) error {
		got = append(got, ev.Type)
		return nil
	})
	if err != nil {
		t.Fatalf("StreamMessage: %v", err)
	}
	want := []string{"start", "chunk", "chunk", "done"}
	if len(got) != len(want) {
		t.Fatalf("got %v want %v", got, want)
	}
	for i := range got {
		if got[i] != want[i] {
			t.Errorf("event %d: got %q want %q", i, got[i], want[i])
		}
	}
}

func TestModelsListsData(t *testing.T) {
	_, c := newServer(t, func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": []map[string]any{
				{"id": "claude-haiku", "name": "Haiku", "provider": "anthropic", "tier": "free", "default": true},
			},
		})
	})

	models, err := c.Models(context.Background())
	if err != nil {
		t.Fatalf("Models: %v", err)
	}
	if len(models) != 1 || models[0].ID != "claude-haiku" {
		t.Errorf("unexpected: %+v", models)
	}
}

func TestAgentTaskCreateClaimSubmit(t *testing.T) {
	var seen []string
	_, c := newServer(t, func(w http.ResponseWriter, r *http.Request) {
		seen = append(seen, r.Method+" "+r.URL.Path)
		if auth := r.Header.Get("Authorization"); auth != "Bearer test-token" {
			t.Errorf("authorization header: %q", auth)
		}
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v1/agent_tasks":
			if r.Method != http.MethodPost {
				t.Errorf("method: %s", r.Method)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"data": map[string]any{"id": "t1", "title": "Smoke", "status": "queued"},
			})
		case "/api/v1/agent_tasks/claim":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"data": map[string]any{"id": "t1", "title": "Smoke", "status": "running", "claimed_by": "runner-1"},
			})
		case "/api/v1/agent_tasks/t1/submit":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"data": map[string]any{"id": "t1", "title": "Smoke", "status": "submitted"},
			})
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	})

	created, err := c.CreateAgentTask(context.Background(), AgentTaskCreate{
		Title:     "Smoke",
		Objective: "Run true",
		Commands:  []AgentTaskCommand{{Run: "true"}},
	})
	if err != nil {
		t.Fatalf("CreateAgentTask: %v", err)
	}
	if created.ID != "t1" || created.Status != "queued" {
		t.Fatalf("created: %+v", created)
	}

	claimed, err := c.ClaimAgentTask(context.Background(), AgentTaskClaim{RunnerID: "runner-1"})
	if err != nil {
		t.Fatalf("ClaimAgentTask: %v", err)
	}
	if claimed.ClaimedBy != "runner-1" {
		t.Fatalf("claimed: %+v", claimed)
	}

	submitted, err := c.SubmitAgentTask(context.Background(), "t1", "runner-1", "green", map[string]any{"status": "passed"})
	if err != nil {
		t.Fatalf("SubmitAgentTask: %v", err)
	}
	if submitted.Status != "submitted" {
		t.Fatalf("submitted: %+v", submitted)
	}

	want := []string{"POST /api/v1/agent_tasks", "POST /api/v1/agent_tasks/claim", "POST /api/v1/agent_tasks/t1/submit"}
	if strings.Join(seen, ",") != strings.Join(want, ",") {
		t.Fatalf("seen %v want %v", seen, want)
	}
}

func TestClaimAgentTaskEmptyQueue(t *testing.T) {
	_, c := newServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/agent_tasks/claim" {
			t.Fatalf("path: %s", r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"data": nil})
	})

	task, err := c.ClaimAgentTask(context.Background(), AgentTaskClaim{RunnerID: "runner-1"})
	if err != nil {
		t.Fatalf("ClaimAgentTask: %v", err)
	}
	if task != nil {
		t.Fatalf("expected nil task, got %+v", task)
	}
}
