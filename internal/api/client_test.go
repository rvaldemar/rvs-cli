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
