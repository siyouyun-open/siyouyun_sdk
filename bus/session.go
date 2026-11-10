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

// frameTransport abstracts the streaming connection transport: it reads and writes one
// message as one frame.
// UDS is "one JSON object per line"; WS is "one JSON object per text frame".
type frameTransport interface {
	Name() string
	writeFrame([]byte) error
	readFrame() ([]byte, error)
	close() error
}

// session performs RPC and event dispatch over a single streaming connection.
//
// Key constraint (see the client architecture in
// skills/mini-packer/references/capability-bus.md):
// on one connection responses may arrive out of order and events interleave, so you cannot
// treat "write a request" followed by "read the next frame" as its response — responses
// must be matched back by ID.
type session struct {
	transport frameTransport
	opts      Options
	events    chan protocol.Event
	// onReliable receives event pushes that carry a delivery id (reliable subscriptions).
	// It must not block: the reader must never stall in-flight calls, so the client buffers
	// these events in its own queue instead of into the bounded "hint" channel.
	onReliable func(protocol.Event)

	writeMu sync.Mutex

	mu      sync.Mutex
	pending map[string]chan *protocol.Response
	nextID  uint64
	failed  *connError

	// ackSeq numbers the one-way ack frames. It is separate from nextID on purpose: acks have
	// no pending entry, and reusing the request-id sequence would collide with a real reply.
	ackSeq atomic.Uint64

	done      chan struct{}
	closeOnce sync.Once
}

func newSession(transport frameTransport, opts Options, events chan protocol.Event, onReliable func(protocol.Event)) *session {
	s := &session{
		transport:  transport,
		opts:       opts,
		events:     events,
		onReliable: onReliable,
		pending:    make(map[string]chan *protocol.Response),
		done:       make(chan struct{}),
	}
	go s.readLoop()
	return s
}

// call issues a capability call and waits for the response.
func (s *session) call(ctx context.Context, req *Request) (*protocol.Response, error) {
	payload, err := encodePayload(req.Payload)
	if err != nil {
		return nil, err
	}
	return s.roundTrip(ctx, protocol.Request{
		Type:          protocol.TypeCall,
		Template:      req.Template,
		Signature:     req.Signature,
		ProviderAppID: req.ProviderAppID,
		Payload:       payload,
		Headers:       req.Headers,
	}, req.Timeout)
}

// subscribe registers topic subscriptions and waits for confirmation.
func (s *session) subscribe(ctx context.Context, topics []string) error {
	_, err := s.roundTrip(ctx, protocol.Request{Type: protocol.TypeSubscribe, Topics: topics}, 0)
	return err
}

// subscribeReliable registers at-least-once topic subscriptions.
//
// The confirmation is part of the contract: the daemon must echo `{"reliable":true}` in
// data. Without it the daemon either predates the ACK protocol or refused the reliable mode,
// and the caller must not be left believing that it will be redelivered (see
// ErrReliableUnsupported).
func (s *session) subscribeReliable(ctx context.Context, topics []string) error {
	resp, err := s.roundTrip(ctx, protocol.Request{Type: protocol.TypeSubscribe, Topics: topics, Reliable: true}, 0)
	if err != nil {
		return err
	}
	if resp != nil && resp.Code != "" && resp.Code != protocol.CodeSuccess {
		msg := strings.TrimSpace(resp.Error)
		if msg == "" {
			msg = "reliable subscribe rejected"
		}
		return fmt.Errorf("bus: reliable subscribe %v rejected (%s): %s", topics, resp.Code, msg)
	}
	var confirmation struct {
		Reliable bool `json:"reliable"`
	}
	if resp != nil && len(resp.Data) > 0 {
		_ = json.Unmarshal(resp.Data, &confirmation)
	}
	if !confirmation.Reliable {
		return fmt.Errorf("%w (topics %v)", ErrReliableUnsupported, topics)
	}
	return nil
}

// ack acknowledges one reliable delivery. It is fire-and-forget by design: when the write
// fails the daemon simply keeps the delivery unacknowledged and redelivers it, which is
// exactly the at-least-once behaviour the caller asked for.
func (s *session) ack(eventID string) error {
	raw, err := json.Marshal(protocol.Request{
		Type:    protocol.TypeAck,
		ID:      fmt.Sprintf("ack-%d", s.ackSeq.Add(1)),
		EventID: eventID,
	})
	if err != nil {
		return fmt.Errorf("bus: encode ack: %w", err)
	}
	return s.write(raw)
}

// unsubscribe cancels subscriptions and waits for confirmation.
func (s *session) unsubscribe(ctx context.Context, topics []string) error {
	_, err := s.roundTrip(ctx, protocol.Request{Type: protocol.TypeUnsubscribe, Topics: topics}, 0)
	return err
}

func (s *session) roundTrip(ctx context.Context, frame protocol.Request, timeout time.Duration) (*protocol.Response, error) {
	if timeout <= 0 {
		timeout = s.opts.CallTimeout
	}
	callCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	id, ch, err := s.register()
	if err != nil {
		return nil, err
	}
	defer s.unregister(id)
	frame.ID = id

	raw, err := json.Marshal(frame)
	if err != nil {
		return nil, fmt.Errorf("bus: encode request: %w", err)
	}
	if err := s.write(raw); err != nil {
		return nil, err
	}

	select {
	case resp := <-ch:
		return resp, nil
	case <-callCtx.Done():
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, fmt.Errorf("%w (%s)", ErrTimeout, frame.Template)
	case <-s.done:
		return nil, s.connErr()
	}
}

func (s *session) write(raw []byte) error {
	select {
	case <-s.done:
		return s.connErr()
	default:
	}
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	if err := s.transport.writeFrame(raw); err != nil {
		s.fail(err)
		return s.connErr()
	}
	return nil
}

// register records a pending request; it rejects when the number of concurrent in-flight
// requests exceeds the limit, avoiding unbounded pile-up.
func (s *session) register() (string, chan *protocol.Response, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.failed != nil {
		return "", nil, s.failed
	}
	if len(s.pending) >= s.opts.PendingLimit {
		return "", nil, errors.New("bus: too many in-flight requests")
	}
	s.nextID++
	id := fmt.Sprintf("%d", s.nextID)
	ch := make(chan *protocol.Response, 1)
	s.pending[id] = ch
	return id, ch, nil
}

func (s *session) unregister(id string) {
	s.mu.Lock()
	delete(s.pending, id)
	s.mu.Unlock()
}

func (s *session) deliver(id string, resp *protocol.Response) {
	s.mu.Lock()
	ch := s.pending[id]
	s.mu.Unlock()
	if ch == nil {
		return // request already timed out or was canceled; drop it
	}
	select {
	case ch <- resp:
	default:
	}
}

// emit pushes an event without blocking: events are hints, not a reliable log, so a full
// channel drops the event rather than blocking the reader
// (blocking the reader would stall every in-flight call).
func (s *session) emit(event protocol.Event) {
	select {
	case s.events <- event:
	default:
		s.opts.Logger("bus: event buffer full, dropping event topic=%s", event.Topic)
	}
}

func (s *session) readLoop() {
	for {
		raw, err := s.transport.readFrame()
		if err != nil {
			s.fail(err)
			return
		}
		var frame protocol.Response
		if err := json.Unmarshal(raw, &frame); err != nil {
			s.opts.Logger("bus: dropping unparsable frame: %v", err)
			continue
		}
		switch frame.Type {
		case protocol.FrameResponse:
			s.deliver(frame.ID, &frame)
		case protocol.FrameEvent:
			event := protocol.Event{Topic: frame.Topic, Data: frame.Event, ID: frame.EventID}
			// A delivery id marks an at-least-once delivery: it goes to the reliable queue
			// (which is accounted and never mixed with the bounded hint channel) and must be
			// acknowledged by the subscriber. Without an id the push keeps the old
			// fire-and-forget semantics.
			if event.ID != "" && s.onReliable != nil {
				s.onReliable(event)
				continue
			}
			s.emit(event)
		case protocol.FramePong:
			// Heartbeat response, nothing to handle.
		default:
			s.opts.Logger("bus: dropping unknown frame type %q", frame.Type)
		}
	}
}

// fail records a connection-level failure and closes the session (both readLoop and write
// may trigger it).
func (s *session) fail(err error) {
	if err == nil {
		err = errors.New("bus: connection closed")
	}
	s.mu.Lock()
	already := s.failed != nil
	if !already {
		s.failed = &connError{err: err}
	}
	s.mu.Unlock()
	if already {
		return
	}
	s.closeOnce.Do(func() { close(s.done) })
	_ = s.transport.close()
}

func (s *session) close() error {
	s.fail(errors.New("bus: session closed by client"))
	return nil
}

func (s *session) connErr() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.failed == nil {
		return &connError{err: errors.New("bus: connection closed")}
	}
	return s.failed
}

func (s *session) errString() string {
	err := s.connErr()
	if err == nil {
		return ""
	}
	return err.Error()
}

// encodePayload encodes the call arguments as JSON; nil means no arguments.
func encodePayload(payload any) (json.RawMessage, error) {
	if payload == nil {
		return nil, nil
	}
	if raw, ok := payload.(json.RawMessage); ok {
		return raw, nil
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("bus: encode payload: %w", err)
	}
	return data, nil
}
