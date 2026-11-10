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
//   - Events come in two flavours: the default fire-and-forget subscription (bounded hint
//     channel, drops when full) and the opt-in reliable subscription (`SubscribeReliable`),
//     where every delivery carries a daemon-assigned event id, waits in its own queue, and
//     is only acknowledged (`Ack`) after the subscriber has processed it. Unacknowledged
//     deliveries are redelivered by the daemon after its AckWait, which is what makes the
//     reliable path at-least-once across disconnects.
package bus

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
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
	// defaultReliableQueueLimit bounds the local backlog of unprocessed reliable deliveries.
	// The daemon already bounds in-flight unacknowledged messages (MaxAckPending, 1024 by
	// default), so this is headroom rather than the normal working set; on overflow the
	// delivery is **not** acknowledged and the daemon redelivers it, so nothing is lost.
	defaultReliableQueueLimit = 4096
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
	// EventBuffer is the event channel buffer of the default (fire-and-forget) subscriptions;
	// when full, events are dropped and counted (events are "hints", not a reliable log).
	// It does **not** apply to reliable subscriptions: those have their own queue
	// (ReliableQueueLimit) and an explicit acknowledge/redeliver contract.
	EventBuffer int
	// ReliableQueueLimit bounds the local backlog of reliable deliveries that have been
	// received but not yet handed to the application. On overflow the delivery is dropped
	// without being acknowledged, so the daemon redelivers it after AckWait; the drop is
	// counted in Client.Stats().ReliableDropped and logged.
	ReliableQueueLimit int

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
	topics  map[string]bool // topic -> reliable (at-least-once); false is fire-and-forget
	closed  bool
	lastErr string
	backoff time.Duration

	httpCall *httpTransport
	events   chan protocol.Event
	closedCh chan struct{}

	// Reliable delivery path. Deliveries carrying an event id are appended here by the
	// session reader and handed to the application by pumpReliable; the reader never blocks
	// (a stalled reader would stall every in-flight call).
	reliableMu     sync.Mutex
	reliableCond   *sync.Cond
	reliableQueue  []protocol.Event
	reliableOut    chan protocol.Event
	reliableClosed bool
	reliableOnce   sync.Once

	stats clientStats
}

// clientStats holds the reliable-delivery counters (all accessed atomically).
type clientStats struct {
	reliableDropped atomic.Uint64
	acksSent        atomic.Uint64
	acksFailed      atomic.Uint64
}

// Stats is a snapshot of the reliable-delivery counters of a client.
type Stats struct {
	// ReliableDropped counts reliable deliveries dropped locally because the queue was full.
	// They were not acknowledged, so the daemon will redeliver them after its AckWait; a
	// steadily increasing value means the application is slower than the event rate.
	ReliableDropped uint64
	// AcksSent is the number of acknowledgements written to the connection.
	AcksSent uint64
	// AcksFailed is the number of acknowledgements that could not be written (for example
	// during a reconnect). The daemon redelivers those deliveries.
	AcksFailed uint64
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
	if opts.ReliableQueueLimit <= 0 {
		opts.ReliableQueueLimit = defaultReliableQueueLimit
	}
	if opts.Logger == nil {
		opts.Logger = func(string, ...any) {}
	}

	c := &Client{
		opts:        opts,
		topics:      make(map[string]bool),
		backoff:     opts.MinBackoff,
		events:      make(chan protocol.Event, opts.EventBuffer),
		closedCh:    make(chan struct{}),
		reliableOut: make(chan protocol.Event),
	}
	c.reliableCond = sync.NewCond(&c.reliableMu)
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

// Events returns the event channel of the default (fire-and-forget) subscriptions. Events
// are pushed in arrival order and may be duplicated or lost (pushes during a connection
// interruption are not resent) — the subscriber must process them idempotently.
//
// Deliveries of reliable subscriptions do not appear here; they are returned by
// ReliableEvents and must be acknowledged with Ack.
func (c *Client) Events() <-chan protocol.Event { return c.events }

// ReliableEvents returns the channel of at-least-once deliveries (subscriptions created by
// SubscribeReliable).
//
// Every event read from this channel carries Event.ID and stays unacknowledged on the daemon
// until Ack is called for it. The channel is unbounded from the reader's point of view: the
// session reader appends to an internal queue and a pump goroutine hands events over, so a
// slow handler never blocks calls or the reconnect logic. The queue itself is bounded by
// Options.ReliableQueueLimit; on overflow the delivery is dropped **without** being
// acknowledged and the daemon redelivers it (counted in Stats().ReliableDropped), so the
// only cost is latency, never a lost event.
//
// Only one consumer should read this channel; delivery order is the arrival order, which the
// bus protocol does not otherwise guarantee to be globally ordered.
//
// The channel is closed when the client is closed.
func (c *Client) ReliableEvents() <-chan protocol.Event {
	c.reliableOnce.Do(func() { go c.pumpReliable() })
	return c.reliableOut
}

// Ack acknowledges the reliable delivery identified by eventID (Event.ID). Call it only
// after the event has been fully processed and persisted: a delivery that is not
// acknowledged is redelivered by the daemon after its AckWait (30s on the v2 daemon), and a
// delivery whose acknowledgement is lost to a disconnect is redelivered as well. Delivery is
// therefore at-least-once and the handler must be idempotent.
//
// Ack is fire-and-forget: the daemon does not reply, and the call returns after the frame has
// been written. It deliberately does not dial a new connection just to acknowledge an old
// delivery — after a reconnect the daemon redelivers the unacknowledged event (with a fresh
// Ack on the new connection) instead.
func (c *Client) Ack(eventID string) error {
	eventID = strings.TrimSpace(eventID)
	if eventID == "" {
		return errors.New("bus: ack requires a non-empty event id")
	}
	c.mu.Lock()
	closed := c.closed
	sess := c.sess
	c.mu.Unlock()
	if closed {
		return errors.New("bus: client is closed")
	}
	if sess == nil {
		c.stats.acksFailed.Add(1)
		return fmt.Errorf("%w: ack event_id=%s (the daemon will redeliver it)", ErrNotConnected, eventID)
	}
	if err := sess.ack(eventID); err != nil {
		c.stats.acksFailed.Add(1)
		c.opts.Logger("bus: ack event_id=%s failed: %v (the daemon will redeliver it)", eventID, err)
		return err
	}
	c.stats.acksSent.Add(1)
	return nil
}

// Stats returns a snapshot of the reliable-delivery counters.
func (c *Client) Stats() Stats {
	return Stats{
		ReliableDropped: c.stats.reliableDropped.Load(),
		AcksSent:        c.stats.acksSent.Load(),
		AcksFailed:      c.stats.acksFailed.Load(),
	}
}

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
//
// This is the default fire-and-forget mode: events arrive on Events, may be dropped when the
// buffer is full, and are not resent after a disconnect. For file events that must not be
// lost, use SubscribeReliable.
func (c *Client) Subscribe(ctx context.Context, topics ...string) error {
	return c.subscribe(ctx, false, topics)
}

// SubscribeReliable subscribes to event topics with at-least-once delivery.
//
// Each event of these topics is then delivered on ReliableEvents with a daemon-assigned
// Event.ID and must be acknowledged with Ack once processed. A delivery that is not
// acknowledged (handler returned an error, process restarted, connection lost) is redelivered
// by the daemon after its AckWait; the daemon also keeps the delivery across reconnects
// because the subscription is replayed on the new connection.
//
// The daemon must confirm the reliable mode (protocol: response data `{"reliable":true}`);
// otherwise ErrReliableUnsupported is returned instead of silently downgrading to
// fire-and-forget. Subscribing to an already-reliable topic is idempotent, and a later plain
// Subscribe does not downgrade it.
func (c *Client) SubscribeReliable(ctx context.Context, topics ...string) error {
	return c.subscribe(ctx, true, topics)
}

func (c *Client) subscribe(ctx context.Context, reliable bool, topics []string) error {
	cleaned := cleanTopics(topics)
	if len(cleaned) == 0 {
		return errors.New("bus: at least one topic is required")
	}
	if reliable {
		// Start the pump before the subscription exists, so a delivery that races the
		// subscribe confirmation is queued and dispatched instead of waiting for a reader.
		c.ReliableEvents()
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
	toSend := make([]string, 0, len(cleaned))
	for _, t := range cleaned {
		current, known := c.topics[t]
		switch {
		case !known:
			c.topics[t] = reliable
			toSend = append(toSend, t)
		case reliable && !current:
			// Upgrade fire-and-forget -> reliable.
			c.topics[t] = true
			toSend = append(toSend, t)
		case reliable:
			// Already reliable: nothing to send (idempotent).
		default:
			// Plain Subscribe on an already-reliable topic must not downgrade it.
		}
	}
	c.mu.Unlock()

	if len(toSend) == 0 {
		return nil
	}
	if reliable {
		return sess.subscribeReliable(ctx, toSend)
	}
	return sess.subscribe(ctx, toSend)
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
	// Wake the reliable pump so it stops and closes ReliableEvents; whatever is still queued
	// stays unacknowledged and is redelivered by the daemon.
	c.reliableMu.Lock()
	c.reliableClosed = true
	c.reliableCond.Broadcast()
	c.reliableMu.Unlock()
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
	// Replay both classes of subscription; reliable ones keep their at-least-once contract,
	// which is also what makes the daemon redeliver everything that was not yet acknowledged
	// when the connection was lost.
	var plainTopics, reliableTopics []string
	for t, reliable := range c.topics {
		if reliable {
			reliableTopics = append(reliableTopics, t)
		} else {
			plainTopics = append(plainTopics, t)
		}
	}
	c.mu.Unlock()

	transport, err := c.openTransport(ctx)
	if err != nil {
		c.recordError(err)
		return nil, err
	}

	sess := newSession(transport, c.opts, c.events, c.enqueueReliable)
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

	if len(plainTopics) > 0 {
		if err := sess.subscribe(ctx, plainTopics); err != nil {
			c.opts.Logger("bus: replay subscriptions failed: %v", err)
		}
	}
	if len(reliableTopics) > 0 {
		if err := sess.subscribeReliable(ctx, reliableTopics); err != nil {
			c.opts.Logger("bus: replay reliable subscriptions failed: %v", err)
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

// enqueueReliable is called by the session reader for every delivery that carries an event
// id. It must never block: blocking here would stall the reader and with it every in-flight
// call. On overflow the delivery is dropped without an ack, so the daemon redelivers it.
func (c *Client) enqueueReliable(event protocol.Event) {
	c.reliableMu.Lock()
	if len(c.reliableQueue) >= c.opts.ReliableQueueLimit {
		c.reliableMu.Unlock()
		c.stats.reliableDropped.Add(1)
		c.opts.Logger("bus: reliable queue full (limit %d), dropped delivery event_id=%s without ack; the daemon will redeliver it after AckWait", c.opts.ReliableQueueLimit, event.ID)
		return
	}
	c.reliableQueue = append(c.reliableQueue, event)
	c.reliableCond.Signal()
	c.reliableMu.Unlock()
}

// pumpReliable hands queued reliable deliveries to ReliableEvents, waiting for the consumer
// when it is slower than the daemon. It exits (and closes the channel) when the client is
// closed; queued but undelivered events stay unacknowledged and are redelivered by the
// daemon.
func (c *Client) pumpReliable() {
	defer close(c.reliableOut)
	for {
		c.reliableMu.Lock()
		for len(c.reliableQueue) == 0 && !c.reliableClosed {
			c.reliableCond.Wait()
		}
		if len(c.reliableQueue) == 0 {
			c.reliableMu.Unlock()
			return
		}
		event := c.reliableQueue[0]
		c.reliableQueue = c.reliableQueue[1:]
		closed := c.reliableClosed
		c.reliableMu.Unlock()
		if closed {
			return
		}
		select {
		case c.reliableOut <- event:
		case <-c.closedCh:
			return
		}
	}
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
