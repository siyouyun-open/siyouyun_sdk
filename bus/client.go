// Package bus is the client for the minictl capability bus.
//
// Design stance: this is the **client of the bus**, not yet another app framework.
//
//   - One connection, one reader: Call and event subscriptions reuse the same streaming
//     connection (UDS or WS); responses are matched back by ID, events are dispatched by
//     topic, and neither blocks the other.
//   - The environment chooses the transport, not the app: the MINI_APP_BUS_SOCKET handed
//     to a native process is preferred; without a socket it falls back to WS; the HTTP
//     channel serves file/streaming capabilities and is the last resort.
//   - Identity is delegated to the server: the client does not declare appCode/namespace;
//     over HTTP/WS it only carries the caller token injected by the daemon.
//   - Disconnects are recoverable: after a disconnect it reconnects with exponential
//     backoff and idempotently replays subscriptions.
package bus

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/siyouyun-open/siyouyun_sdk/v2/protocol"
)

// Defaults.
const (
	DefaultCallTimeout  = 30 * time.Second
	DefaultDialTimeout  = 5 * time.Second
	DefaultMinBackoff   = 500 * time.Millisecond
	DefaultMaxBackoff   = 30 * time.Second
	defaultPendingLimit = 256
	defaultEventBuffer  = 256
)

// Options holds the constructor parameters for Client. The zero value is usable: every
// field has a default.
type Options struct {
	// SocketPath is the absolute path of the bus UDS (MINI_APP_BUS_SOCKET).
	SocketPath string
	// WSURL is the bus WebSocket address (MINI_APP_BUS_WS).
	WSURL string
	// HTTPURL is the daemon bus HTTP base address (MINI_APP_BUS_HTTP).
	HTTPURL string

	// CallerToken is the caller token issued by the daemon (MINI_APP_CALLER_TOKEN).
	// It is only used as proof of identity over HTTP/WS; over UDS identity is determined
	// by the socket and its binding to the instance.
	CallerToken string

	// CallTimeout is the default timeout for a single call (a shorter caller ctx wins).
	CallTimeout time.Duration
	// DialTimeout is the dial timeout.
	DialTimeout time.Duration
	// MinBackoff / MaxBackoff bound the reconnect backoff.
	MinBackoff time.Duration
	MaxBackoff time.Duration
	// PendingLimit caps the number of concurrent in-flight requests to prevent unbounded
	// pile-up.
	PendingLimit int
	// EventBuffer is the event channel buffer; when full, events are dropped and counted
	// (events are "hints", not a reliable log).
	EventBuffer int

	// OnStateChange is invoked when the connection is established or lost, so the app can
	// expose its health state.
	OnStateChange func(State)
	// Logger is an optional logger (discarded by default).
	Logger func(format string, args ...any)
}

// State is a snapshot of the client connection state.
type State struct {
	Transport string
	Connected bool
	LastError string
}

// Request is a single capability call.
type Request struct {
	// Template is the capability template (protocol.Template*).
	Template string
	// Signature is the signature of a custom-domain (custom.*) capability; built-in
	// templates do not need it.
	Signature string
	// ProviderAppID pins the call to a specific provider app (optional).
	ProviderAppID string
	// Payload is encoded as JSON; nil means no arguments.
	Payload any
	// Headers are forwarded to the provider; they must not be used to forge identity
	// headers.
	Headers map[string]string
	// Timeout overrides the default timeout for this call (0 means Options.CallTimeout).
	Timeout time.Duration
}

// Client is the bus client.
type Client struct {
	opts Options

	mu      sync.Mutex
	sess    *session
	topics  map[string]struct{}
	closed  bool
	lastErr string
	backoff time.Duration

	httpCall *httpTransport
	events   chan protocol.Event
	closedCh chan struct{}
}

// New creates a client. It does not connect immediately: the connection is established on
// the first Call / Subscribe, so an app can still start and degrade on its own even when
// the bus is temporarily unavailable.
func New(opts Options) *Client {
	if opts.CallTimeout <= 0 {
		opts.CallTimeout = DefaultCallTimeout
	}
	if opts.DialTimeout <= 0 {
		opts.DialTimeout = DefaultDialTimeout
	}
	if opts.MinBackoff <= 0 {
		opts.MinBackoff = DefaultMinBackoff
	}
	if opts.MaxBackoff <= 0 {
		opts.MaxBackoff = DefaultMaxBackoff
	}
	if opts.PendingLimit <= 0 {
		opts.PendingLimit = defaultPendingLimit
	}
	if opts.EventBuffer <= 0 {
		opts.EventBuffer = defaultEventBuffer
	}
	if opts.Logger == nil {
		opts.Logger = func(string, ...any) {}
	}

	c := &Client{
		opts:     opts,
		topics:   make(map[string]struct{}),
		backoff:  opts.MinBackoff,
		events:   make(chan protocol.Event, opts.EventBuffer),
		closedCh: make(chan struct{}),
	}
	if strings.TrimSpace(opts.HTTPURL) != "" {
		c.httpCall = newHTTPTransport(opts.HTTPURL, opts.CallerToken, opts.CallTimeout)
	}
	return c
}

// State returns the current connection state.
func (c *Client) State() State {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.stateLocked()
}

func (c *Client) stateLocked() State {
	name := "none"
	connected := false
	if c.sess != nil {
		name = c.sess.transport.Name()
		connected = true
	} else if c.httpCall != nil {
		name = "http"
	}
	return State{Transport: name, Connected: connected, LastError: c.lastErr}
}

// Events returns the event channel. Events are pushed in arrival order and may be
// duplicated or lost (pushes during a connection interruption are not resent) — the
// subscriber must process them idempotently.
func (c *Client) Events() <-chan protocol.Event { return c.events }

// Call invokes a capability template and returns the raw response.
func (c *Client) Call(ctx context.Context, req *Request) (*protocol.Response, error) {
	if req == nil || strings.TrimSpace(req.Template) == "" {
		return nil, errors.New("bus: template is required")
	}

	sess, err := c.session(ctx)
	if err != nil {
		// Fall back to the HTTP capability channel when there is no streaming connection.
		if c.httpCall != nil {
			return c.httpCall.call(ctx, req)
		}
		return nil, err
	}

	resp, err := sess.call(ctx, req)
	if err == nil {
		return resp, nil
	}
	if !isConnError(err) {
		return nil, err
	}

	// Connection-level failure: drop the bad connection and retry once (this may be a
	// reconnect window).
	c.dropSession(sess, err)
	if sess2, derr := c.session(ctx); derr == nil {
		if resp, rerr := sess2.call(ctx, req); rerr == nil {
			return resp, nil
		}
	}
	if c.httpCall != nil {
		return c.httpCall.call(ctx, req)
	}
	return nil, err
}

// CallInto invokes a capability and decodes the response data into out (when out is nil it
// only verifies success).
func (c *Client) CallInto(ctx context.Context, req *Request, out any) error {
	resp, err := c.Call(ctx, req)
	if err != nil {
		return err
	}
	if resp.Code != protocol.CodeSuccess {
		msg := strings.TrimSpace(resp.Error)
		if msg == "" {
			msg = "capability call failed"
		}
		return &CallError{Template: req.Template, Code: resp.Code, Message: msg}
	}
	if out == nil || len(resp.Data) == 0 {
		return nil
	}
	if err := json.Unmarshal(resp.Data, out); err != nil {
		return fmt.Errorf("bus: decode response for %s: %w", req.Template, err)
	}
	return nil
}

// CallTemplate is the convenience form of Call (no signature, no provider binding).
func (c *Client) CallTemplate(ctx context.Context, template string, payload any) (*protocol.Response, error) {
	return c.Call(ctx, &Request{Template: template, Payload: payload})
}

// CallTemplateInto is the convenience form of CallInto.
func (c *Client) CallTemplateInto(ctx context.Context, template string, payload, out any) error {
	return c.CallInto(ctx, &Request{Template: template, Payload: payload}, out)
}

// Subscribe subscribes to event topics (idempotent). The daemon expands
// system.file.event into the caller's own scope, so the client should subscribe to the
// base topic.
func (c *Client) Subscribe(ctx context.Context, topics ...string) error {
	cleaned := cleanTopics(topics)
	if len(cleaned) == 0 {
		return errors.New("bus: at least one topic is required")
	}

	// Ensure the connection exists before registering the topics: doing it the other way
	// around makes dial's replay logic send the same subscription twice (once as a
	// "replay" and once as this call); the server is idempotent, but it produces
	// meaningless duplicate frames.
	sess, err := c.session(ctx)
	if err != nil {
		return err
	}

	c.mu.Lock()
	for _, t := range cleaned {
		c.topics[t] = struct{}{}
	}
	c.mu.Unlock()

	return sess.subscribe(ctx, cleaned)
}

// Unsubscribe cancels subscriptions (and removes them from the reconnect replay set).
func (c *Client) Unsubscribe(ctx context.Context, topics ...string) error {
	cleaned := cleanTopics(topics)
	if len(cleaned) == 0 {
		return nil
	}
	c.mu.Lock()
	for _, t := range cleaned {
		delete(c.topics, t)
	}
	sess := c.sess
	c.mu.Unlock()
	if sess == nil {
		return nil
	}
	return sess.unsubscribe(ctx, cleaned)
}

// Close closes the client; later calls will fail.
func (c *Client) Close() error {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return nil
	}
	c.closed = true
	sess := c.sess
	c.sess = nil
	c.mu.Unlock()

	close(c.closedCh)
	if sess != nil {
		_ = sess.close()
	}
	return nil
}

// session returns a usable streaming connection, dialing if necessary.
func (c *Client) session(ctx context.Context) (*session, error) {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return nil, errors.New("bus: client is closed")
	}
	if c.sess != nil {
		sess := c.sess
		c.mu.Unlock()
		return sess, nil
	}
	c.mu.Unlock()
	return c.dial(ctx)
}

// dial establishes the streaming connection, then starts the supervisor goroutine and
// replays subscriptions.
func (c *Client) dial(ctx context.Context) (*session, error) {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return nil, errors.New("bus: client is closed")
	}
	if c.sess != nil {
		sess := c.sess
		c.mu.Unlock()
		return sess, nil
	}
	topics := make([]string, 0, len(c.topics))
	for t := range c.topics {
		topics = append(topics, t)
	}
	c.mu.Unlock()

	transport, err := c.openTransport(ctx)
	if err != nil {
		c.recordError(err)
		return nil, err
	}

	sess := newSession(transport, c.opts, c.events)
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		_ = transport.close()
		return nil, errors.New("bus: client is closed")
	}
	c.sess = sess
	c.lastErr = ""
	c.backoff = c.opts.MinBackoff
	c.mu.Unlock()
	c.emitState()

	go c.supervise(sess)

	if len(topics) > 0 {
		if err := sess.subscribe(ctx, topics); err != nil {
			c.opts.Logger("bus: replay subscriptions failed: %v", err)
		}
	}
	return sess, nil
}

// openTransport picks the streaming transport by priority: UDS first (fastest on the local
// machine, supports custom signatures), then WS.
func (c *Client) openTransport(ctx context.Context) (frameTransport, error) {
	dialCtx, cancel := context.WithTimeout(ctx, c.opts.DialTimeout)
	defer cancel()

	if path := strings.TrimSpace(c.opts.SocketPath); path != "" {
		transport, err := dialUnix(dialCtx, path)
		if err == nil {
			return transport, nil
		}
		c.opts.Logger("bus: unix dial %s failed: %v", path, err)
		if strings.TrimSpace(c.opts.WSURL) == "" {
			return nil, err
		}
	}
	if url := strings.TrimSpace(c.opts.WSURL); url != "" {
		return dialWS(dialCtx, url, c.opts.CallerToken)
	}
	return nil, errors.New("bus: no stream transport configured (set MINI_APP_BUS_SOCKET or MINI_APP_BUS_WS)")
}

// supervise watches for connection death and reconnects with backoff; it exits when the
// client is closed or the session has been replaced.
func (c *Client) supervise(sess *session) {
	for {
		select {
		case <-sess.done:
		case <-c.closedCh:
			return
		}

		c.mu.Lock()
		if c.closed {
			c.mu.Unlock()
			return
		}
		// Only exit when the session "has already been replaced by another live
		// connection": that means some other goroutine established a new session and its
		// own supervisor will take over reconnection.
		//
		// This used to read `c.sess != sess`, so **the first failed reconnect** (c.sess
		// had been set to nil) made the loop satisfy that condition and return immediately
		// — the client then never tried again and the app never received another event.
		// Observed on a real machine: after a daemon restart the app log held a single
		// "reconnect failed" and then nothing (measured 2026-09-22, see
		// docs/specs/file-event-permission-model.md §11 step 6).
		if c.sess != nil && c.sess != sess {
			c.mu.Unlock()
			return
		}
		c.sess = nil
		c.lastErr = sess.errString()
		delay := c.backoff
		if c.backoff < c.opts.MaxBackoff {
			c.backoff *= 2
			if c.backoff > c.opts.MaxBackoff {
				c.backoff = c.opts.MaxBackoff
			}
		}
		c.mu.Unlock()
		c.emitState()

		c.opts.Logger("bus: connection lost (%s), reconnecting in %s", sess.errString(), delay)
		select {
		case <-time.After(delay):
		case <-c.closedCh:
			return
		}

		ctx, cancel := context.WithTimeout(context.Background(), c.opts.DialTimeout)
		_, err := c.dial(ctx)
		cancel()
		if err == nil {
			c.opts.Logger("bus: reconnected")
			return
		}
		c.opts.Logger("bus: reconnect failed: %v", err)
	}
}

// dropSession proactively discards a broken connection.
func (c *Client) dropSession(sess *session, cause error) {
	c.mu.Lock()
	if c.sess == sess {
		c.sess = nil
		c.lastErr = cause.Error()
	}
	c.mu.Unlock()
	_ = sess.close()
	c.emitState()
}

func (c *Client) recordError(err error) {
	if err == nil {
		return
	}
	c.mu.Lock()
	c.lastErr = err.Error()
	c.mu.Unlock()
	c.emitState()
}

func (c *Client) emitState() {
	if c.opts.OnStateChange == nil {
		return
	}
	c.mu.Lock()
	state := c.stateLocked()
	c.mu.Unlock()
	c.opts.OnStateChange(state)
}

func cleanTopics(topics []string) []string {
	cleaned := make([]string, 0, len(topics))
	seen := make(map[string]struct{}, len(topics))
	for _, t := range topics {
		t = strings.TrimSpace(t)
		if t == "" {
			continue
		}
		if _, dup := seen[t]; dup {
			continue
		}
		seen[t] = struct{}{}
		cleaned = append(cleaned, t)
	}
	return cleaned
}
