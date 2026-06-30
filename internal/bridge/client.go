// Package bridge implements an ActionCable WebSocket client that tunnels
// Openclaude proto3-JSON messages between the local CLI and the Hub's
// CodeBridgeChannel.
//
// The Hub speaks Rails 8 ActionCable on /cable. After upgrade, frames are
// JSON envelopes following the documented ActionCable wire protocol:
//
//	server → client:
//	  {"type":"welcome"}
//	  {"type":"ping","message":<unix-ts>}
//	  {"type":"confirm_subscription","identifier":"<id-json>"}
//	  {"type":"reject_subscription","identifier":"<id-json>"}
//	  {"identifier":"<id-json>","message":<channel-payload>}
//	  {"type":"disconnect","reason":"..."}
//
//	client → server:
//	  {"command":"subscribe","identifier":"<id-json>"}
//	  {"command":"message","identifier":"<id-json>","data":"<json-string>"}
//	  {"command":"unsubscribe","identifier":"<id-json>"}
//
// `identifier` is a JSON-encoded string (channel descriptor).
//
// On top of that, CodeBridgeChannel emits three message shapes:
//
//	{"ready":   {"session_id":"<uuid>"}}
//	{"payload": {<proto3-JSON of openclaude.ServerMessage>}}
//	{"error":   {"type":"<code>","message":"...", ...extras}}
//
// And accepts one outbound action:
//
//	{"action":"forward","payload":{<proto3-JSON of openclaude.ClientMessage>}}
package bridge

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"google.golang.org/protobuf/encoding/protojson"

	openclaudev1 "github.com/rvaldemar/rvs-cli/internal/openclaude/v1"
)

const (
	channelName = "CodeBridgeChannel"
	subprotocol = "actioncable-v1-json"

	dialTimeout       = 30 * time.Second
	subscribeTimeout  = 30 * time.Second
	writeWaitTimeout  = 10 * time.Second
	defaultPongDeadln = 90 * time.Second
)

// EventKind enumerates the channel-level message shapes the Hub emits.
type EventKind int

const (
	// EventReady arrives once after a successful subscribe. The session id
	// is informational — the Hub manages session state on the sidecar.
	EventReady EventKind = iota
	// EventPayload wraps an openclaude ServerMessage from the sidecar.
	EventPayload
	// EventError signals a Hub-level failure (quota, auth, decode, timeout, ...).
	// After EventError the connection is typically closed by the Hub.
	EventError
)

// ReadyData is the body of a `ready` frame.
type ReadyData struct {
	SessionID string `json:"session_id"`
}

// ErrorData is the body of an `error` frame. Type is a stable code
// (quota_exceeded, access_error, invalid_request_error, timeout, ...).
// Extras carries the rest of the JSON object so callers can surface details.
type ErrorData struct {
	Type    string                 `json:"type"`
	Message string                 `json:"message"`
	Extras  map[string]interface{} `json:"-"`
}

// Event is yielded on the channel returned by Events.
type Event struct {
	Kind    EventKind
	Ready   *ReadyData
	Payload *openclaudev1.ServerMessage
	Error   *ErrorData
}

// Client is a single-shot ActionCable client subscribed to CodeBridgeChannel.
// It is not safe for concurrent Send from multiple goroutines (writes are
// serialized through a mutex internally, but the public contract assumes a
// single producer).
type Client struct {
	apiBase string
	token   string

	conn       *websocket.Conn
	identifier string

	writeMu sync.Mutex

	events    chan Event
	closeOnce sync.Once
	closed    chan struct{}

	sessionID string
}

// New returns a Client bound to apiBase ("https://agents.rvs.solutions") and
// the given CliToken. apiBase is the same value the rest of the CLI uses.
func New(apiBase, token string) *Client {
	return &Client{
		apiBase: strings.TrimRight(apiBase, "/"),
		token:   token,
		events:  make(chan Event, 16),
		closed:  make(chan struct{}),
	}
}

// Connect dials the Hub's /cable endpoint, subscribes to CodeBridgeChannel,
// and waits for the `ready` envelope. Returns when the channel is ready to
// accept Send calls. The reader loop is started before Connect returns.
//
// Authentication: the CliToken is sent only in the Authorization header. The
// token must never be placed in the WebSocket URL because URLs are routinely
// logged by proxies, Rails, and diagnostics.
func (c *Client) Connect(ctx context.Context) error {
	u, err := c.cableURL()
	if err != nil {
		return err
	}

	header := http.Header{
		"User-Agent":    {"rvs-cli"},
		"Authorization": {"Bearer " + c.token},
	}

	dialer := websocket.Dialer{
		Subprotocols:     []string{subprotocol},
		HandshakeTimeout: dialTimeout,
	}

	dctx, cancel := context.WithTimeout(ctx, dialTimeout)
	defer cancel()

	conn, resp, err := dialer.DialContext(dctx, u.String(), header)
	if err != nil {
		if resp != nil {
			return fmt.Errorf("dial cable: %s (HTTP %d)", err, resp.StatusCode)
		}
		return fmt.Errorf("dial cable: %w", err)
	}
	c.conn = conn

	c.identifier = encodeIdentifier(channelName)

	// ActionCable sends {type:"welcome"} immediately. Read it before sending
	// subscribe so a server reject lands on the right state.
	if err := c.expectWelcome(ctx); err != nil {
		_ = c.conn.Close()
		return err
	}

	if err := c.sendCommand(cableOutbound{Command: "subscribe", Identifier: c.identifier}); err != nil {
		_ = c.conn.Close()
		return fmt.Errorf("send subscribe: %w", err)
	}

	go c.readLoop()

	// Wait for either confirm_subscription or reject_subscription. The
	// CodeBridgeChannel transmits a `ready` envelope (or an `error` envelope
	// + reject) immediately after subscribing — surface the first one.
	if err := c.awaitReady(ctx); err != nil {
		c.Close()
		return err
	}
	return nil
}

func (c *Client) cableURL() (*url.URL, error) {
	u, err := url.Parse(c.apiBase + "/cable")
	if err != nil {
		return nil, fmt.Errorf("parse cable url: %w", err)
	}
	switch u.Scheme {
	case "http":
		u.Scheme = "ws"
	case "https":
		u.Scheme = "wss"
	case "ws", "wss":
		// pre-set, leave alone
	default:
		return nil, fmt.Errorf("unsupported scheme %q on cable url", u.Scheme)
	}
	return u, nil
}

func (c *Client) expectWelcome(ctx context.Context) error {
	deadline := time.Now().Add(subscribeTimeout)
	if d, ok := ctx.Deadline(); ok && d.Before(deadline) {
		deadline = d
	}
	if err := c.conn.SetReadDeadline(deadline); err != nil {
		return err
	}
	defer func() { _ = c.conn.SetReadDeadline(time.Time{}) }()

	var frame cableInbound
	if err := c.conn.ReadJSON(&frame); err != nil {
		return fmt.Errorf("read welcome: %w", err)
	}
	switch frame.Type {
	case "welcome":
		return nil
	case "disconnect":
		return fmt.Errorf("server disconnected before welcome: %s", frame.Reason)
	default:
		return fmt.Errorf("unexpected pre-welcome frame: type=%q", frame.Type)
	}
}

// awaitReady consumes Events emitted by readLoop until the first Ready, Error
// arrives, or the context expires. Other events queued in the meantime are
// preserved on the channel.
func (c *Client) awaitReady(ctx context.Context) error {
	timeout := time.NewTimer(subscribeTimeout)
	defer timeout.Stop()

	for {
		select {
		case ev, ok := <-c.events:
			if !ok {
				return errors.New("connection closed before ready")
			}
			switch ev.Kind {
			case EventReady:
				if ev.Ready != nil {
					c.sessionID = ev.Ready.SessionID
				}
				return nil
			case EventError:
				if ev.Error != nil {
					return fmt.Errorf("subscribe rejected: %s — %s", ev.Error.Type, ev.Error.Message)
				}
				return errors.New("subscribe rejected: unknown error")
			default:
				// payload arriving before ready is unexpected but not fatal;
				// re-queue would deadlock, so just ignore.
			}
		case <-timeout.C:
			return errors.New("timed out waiting for ready envelope")
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

// SessionID returns the session id reported by the Hub on the ready frame.
// Returns "" if Connect has not completed.
func (c *Client) SessionID() string { return c.sessionID }

// Events returns the channel of Hub-level events. The channel is closed when
// the connection ends (graceful close, server disconnect, or transport error).
func (c *Client) Events() <-chan Event { return c.events }

// Send marshals the proto message to proto3-JSON and forwards it through the
// CodeBridgeChannel `forward` action.
func (c *Client) Send(msg *openclaudev1.ClientMessage) error {
	if msg == nil {
		return errors.New("nil ClientMessage")
	}
	payload, err := protojson.Marshal(msg)
	if err != nil {
		return fmt.Errorf("marshal client message: %w", err)
	}

	// ActionCable's `data` field is itself a JSON string. We embed the proto
	// JSON as a parsed object rather than a string so the Hub sees a Hash on
	// `data["payload"]` (matches what `Openclaude::V1::ClientMessage.decode_json`
	// expects after to_json round-trip).
	var payloadObj json.RawMessage = payload
	dataBytes, err := json.Marshal(map[string]any{
		"action":  "forward",
		"payload": payloadObj,
	})
	if err != nil {
		return fmt.Errorf("marshal forward action: %w", err)
	}

	return c.sendCommand(cableOutbound{
		Command:    "message",
		Identifier: c.identifier,
		Data:       string(dataBytes),
	})
}

// Close unsubscribes and closes the WebSocket. Safe to call multiple times.
func (c *Client) Close() error {
	c.closeOnce.Do(func() {
		if c.conn != nil {
			_ = c.sendCommand(cableOutbound{Command: "unsubscribe", Identifier: c.identifier})
			_ = c.conn.WriteControl(
				websocket.CloseMessage,
				websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""),
				time.Now().Add(writeWaitTimeout),
			)
			_ = c.conn.Close()
		}
		close(c.closed)
	})
	return nil
}

// sendCommand serializes a control frame to JSON and writes it. Synchronized
// because gorilla/websocket forbids concurrent writers on the same conn.
func (c *Client) sendCommand(out cableOutbound) error {
	c.writeMu.Lock()
	defer c.writeMu.Unlock()

	if c.conn == nil {
		return errors.New("not connected")
	}
	if err := c.conn.SetWriteDeadline(time.Now().Add(writeWaitTimeout)); err != nil {
		return err
	}
	return c.conn.WriteJSON(out)
}

// readLoop translates ActionCable frames into channel-level Events.
func (c *Client) readLoop() {
	defer close(c.events)

	if err := c.conn.SetReadDeadline(time.Now().Add(defaultPongDeadln)); err != nil {
		return
	}

	for {
		select {
		case <-c.closed:
			return
		default:
		}

		var frame cableInbound
		if err := c.conn.ReadJSON(&frame); err != nil {
			if !isExpectedClose(err) {
				c.events <- Event{Kind: EventError, Error: &ErrorData{
					Type:    "transport_error",
					Message: err.Error(),
				}}
			}
			return
		}

		// Refresh the read deadline on every frame — pings keep us alive.
		_ = c.conn.SetReadDeadline(time.Now().Add(defaultPongDeadln))

		switch frame.Type {
		case "ping":
			continue
		case "welcome", "confirm_subscription":
			continue
		case "reject_subscription":
			c.events <- Event{Kind: EventError, Error: &ErrorData{
				Type:    "subscription_rejected",
				Message: "Hub rejected CodeBridgeChannel subscription",
			}}
			return
		case "disconnect":
			c.events <- Event{Kind: EventError, Error: &ErrorData{
				Type:    "disconnected",
				Message: frame.Reason,
			}}
			return
		}

		// Channel data frame — frame.Identifier identifies the channel and
		// frame.Message is the transmit payload.
		if len(frame.Message) == 0 {
			continue
		}
		c.dispatchMessage(frame.Message)
	}
}

func (c *Client) dispatchMessage(raw json.RawMessage) {
	var envelope struct {
		Ready   json.RawMessage `json:"ready,omitempty"`
		Payload json.RawMessage `json:"payload,omitempty"`
		Error   json.RawMessage `json:"error,omitempty"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		c.events <- Event{Kind: EventError, Error: &ErrorData{
			Type:    "decode_error",
			Message: fmt.Sprintf("malformed channel frame: %v", err),
		}}
		return
	}

	switch {
	case len(envelope.Ready) > 0:
		var r ReadyData
		_ = json.Unmarshal(envelope.Ready, &r)
		c.events <- Event{Kind: EventReady, Ready: &r}

	case len(envelope.Payload) > 0:
		msg := &openclaudev1.ServerMessage{}
		if err := protojson.Unmarshal(envelope.Payload, msg); err != nil {
			c.events <- Event{Kind: EventError, Error: &ErrorData{
				Type:    "decode_error",
				Message: fmt.Sprintf("server message: %v", err),
			}}
			return
		}
		c.events <- Event{Kind: EventPayload, Payload: msg}

	case len(envelope.Error) > 0:
		errData := decodeError(envelope.Error)
		c.events <- Event{Kind: EventError, Error: errData}

	default:
		// Unknown envelope shape — ignore, do not break the stream.
	}
}

func decodeError(raw json.RawMessage) *ErrorData {
	var generic map[string]interface{}
	if err := json.Unmarshal(raw, &generic); err != nil {
		return &ErrorData{Type: "decode_error", Message: err.Error()}
	}
	out := &ErrorData{}
	if v, ok := generic["type"].(string); ok {
		out.Type = v
		delete(generic, "type")
	}
	if v, ok := generic["message"].(string); ok {
		out.Message = v
		delete(generic, "message")
	}
	if len(generic) > 0 {
		out.Extras = generic
	}
	return out
}

// === framing types ===

type cableInbound struct {
	Type       string          `json:"type,omitempty"`
	Identifier string          `json:"identifier,omitempty"`
	Message    json.RawMessage `json:"message,omitempty"`
	Reason     string          `json:"reason,omitempty"`
}

type cableOutbound struct {
	Command    string `json:"command"`
	Identifier string `json:"identifier"`
	Data       string `json:"data,omitempty"`
}

func encodeIdentifier(channel string) string {
	id, _ := json.Marshal(map[string]string{"channel": channel})
	return string(id)
}

func isExpectedClose(err error) bool {
	if err == nil {
		return true
	}
	if websocket.IsCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway) {
		return true
	}
	return false
}
