package bridge

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"google.golang.org/protobuf/encoding/protojson"

	openclaudev1 "github.com/rvaldemar/rvs-cli/internal/openclaude/v1"
)

// fakeCable is a minimal ActionCable server for tests. It accepts a single
// subscribe to CodeBridgeChannel, delegates to handler() for the message
// stream, and never sends pings.
type fakeCable struct {
	t       *testing.T
	handler func(*websocket.Conn, string)
}

func (s *fakeCable) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.URL.Query().Get("cli_token") != "" {
		s.t.Errorf("cli_token must not be sent in the WebSocket URL")
	}
	if got := r.Header.Get("Authorization"); !strings.HasPrefix(got, "Bearer ") {
		s.t.Errorf("missing bearer Authorization header: %q", got)
	}

	upgrader := websocket.Upgrader{
		Subprotocols:    []string{"actioncable-v1-json"},
		CheckOrigin:     func(*http.Request) bool { return true },
		ReadBufferSize:  4096,
		WriteBufferSize: 4096,
	}
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		s.t.Fatalf("upgrade: %v", err)
	}
	defer conn.Close()

	if err := conn.WriteJSON(map[string]string{"type": "welcome"}); err != nil {
		return
	}

	var sub cableOutbound
	if err := conn.ReadJSON(&sub); err != nil {
		return
	}
	if sub.Command != "subscribe" {
		s.t.Errorf("expected subscribe, got %q", sub.Command)
	}

	if err := conn.WriteJSON(map[string]string{
		"type":       "confirm_subscription",
		"identifier": sub.Identifier,
	}); err != nil {
		return
	}

	s.handler(conn, sub.Identifier)
}

func startFakeCable(t *testing.T, handler func(conn *websocket.Conn, identifier string)) *httptest.Server {
	srv := httptest.NewServer(&fakeCable{t: t, handler: handler})
	t.Cleanup(srv.Close)
	return srv
}

func TestClient_ConnectReady(t *testing.T) {
	srv := startFakeCable(t, func(conn *websocket.Conn, id string) {
		_ = conn.WriteJSON(map[string]any{
			"identifier": id,
			"message":    map[string]any{"ready": map[string]string{"session_id": "s-123"}},
		})
		// keep open until client closes
		_, _, _ = conn.ReadMessage()
	})

	c := New(srv.URL, "test-token")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := c.Connect(ctx); err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer c.Close()

	if c.SessionID() != "s-123" {
		t.Errorf("session id: got %q want %q", c.SessionID(), "s-123")
	}
}

func TestClient_Connect_RejectsOnError(t *testing.T) {
	srv := startFakeCable(t, func(conn *websocket.Conn, id string) {
		_ = conn.WriteJSON(map[string]any{
			"identifier": id,
			"message":    map[string]any{"error": map[string]any{"type": "quota_exceeded", "message": "over"}},
		})
		_, _, _ = conn.ReadMessage()
	})

	c := New(srv.URL, "tok")
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	err := c.Connect(ctx)
	if err == nil {
		t.Fatalf("expected connect to fail on error envelope")
	}
	if !strings.Contains(err.Error(), "quota_exceeded") {
		t.Errorf("expected quota_exceeded in error, got %v", err)
	}
}

func TestClient_SendForwardsClientMessage(t *testing.T) {
	var (
		mu       sync.Mutex
		received *openclaudev1.ClientMessage
	)

	srv := startFakeCable(t, func(conn *websocket.Conn, id string) {
		_ = conn.WriteJSON(map[string]any{
			"identifier": id,
			"message":    map[string]any{"ready": map[string]string{"session_id": "s-1"}},
		})
		var frame cableOutbound
		if err := conn.ReadJSON(&frame); err != nil {
			return
		}
		var data struct {
			Action  string          `json:"action"`
			Payload json.RawMessage `json:"payload"`
		}
		if err := json.Unmarshal([]byte(frame.Data), &data); err != nil {
			return
		}
		msg := &openclaudev1.ClientMessage{}
		if err := protojson.Unmarshal(data.Payload, msg); err == nil {
			mu.Lock()
			received = msg
			mu.Unlock()
		}
		_, _, _ = conn.ReadMessage()
	})

	c := New(srv.URL, "tok")
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := c.Connect(ctx); err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer c.Close()

	out := &openclaudev1.ClientMessage{
		Payload: &openclaudev1.ClientMessage_Request{
			Request: &openclaudev1.ChatRequest{
				Message:          "hello",
				WorkingDirectory: "/tmp",
				SessionId:        "s-1",
			},
		},
	}
	if err := c.Send(out); err != nil {
		t.Fatalf("send: %v", err)
	}

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		mu.Lock()
		got := received
		mu.Unlock()
		if got != nil {
			req := got.GetRequest()
			if req == nil {
				t.Fatalf("expected request payload, got %+v", got)
			}
			if req.GetMessage() != "hello" {
				t.Errorf("message: got %q", req.GetMessage())
			}
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("server never received the forwarded message")
}

func TestClient_Events_DeliversPayloads(t *testing.T) {
	srv := startFakeCable(t, func(conn *websocket.Conn, id string) {
		_ = conn.WriteJSON(map[string]any{
			"identifier": id,
			"message":    map[string]any{"ready": map[string]string{"session_id": "s-x"}},
		})

		// Send a TextChunk server message.
		serverMsg := &openclaudev1.ServerMessage{
			Event: &openclaudev1.ServerMessage_TextChunk{
				TextChunk: &openclaudev1.TextChunk{Text: "hi there"},
			},
		}
		raw, _ := protojson.Marshal(serverMsg)
		var asObj map[string]any
		_ = json.Unmarshal(raw, &asObj)
		_ = conn.WriteJSON(map[string]any{
			"identifier": id,
			"message":    map[string]any{"payload": asObj},
		})

		// Hold until client closes.
		_, _, _ = conn.ReadMessage()
	})

	c := New(srv.URL, "tok")
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := c.Connect(ctx); err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer c.Close()

	select {
	case ev, ok := <-c.Events():
		if !ok {
			t.Fatal("events closed unexpectedly")
		}
		if ev.Kind != EventPayload {
			t.Fatalf("expected EventPayload, got %v", ev.Kind)
		}
		text := ev.Payload.GetTextChunk().GetText()
		if text != "hi there" {
			t.Errorf("text: got %q want %q", text, "hi there")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for payload event")
	}
}

func TestClient_Events_DeliversErrorEnvelope(t *testing.T) {
	srv := startFakeCable(t, func(conn *websocket.Conn, id string) {
		_ = conn.WriteJSON(map[string]any{
			"identifier": id,
			"message":    map[string]any{"ready": map[string]string{"session_id": "s-y"}},
		})
		_ = conn.WriteJSON(map[string]any{
			"identifier": id,
			"message": map[string]any{"error": map[string]any{
				"type":    "bridge_error",
				"message": "kaboom",
				"detail":  "extra info",
			}},
		})
		_, _, _ = conn.ReadMessage()
	})

	c := New(srv.URL, "tok")
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := c.Connect(ctx); err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer c.Close()

	select {
	case ev := <-c.Events():
		if ev.Kind != EventError {
			t.Fatalf("expected EventError, got %v", ev.Kind)
		}
		if ev.Error.Type != "bridge_error" || ev.Error.Message != "kaboom" {
			t.Errorf("unexpected error envelope: %+v", ev.Error)
		}
		if got, _ := ev.Error.Extras["detail"].(string); got != "extra info" {
			t.Errorf("extras lost: %v", ev.Error.Extras)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for error event")
	}
}
