package bus

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
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

	writeMu sync.Mutex

	mu      sync.Mutex
	pending map[string]chan *protocol.Response
	nextID  uint64
	failed  *connError

	done      chan struct{}
	closeOnce sync.Once
}

func newSession(transport frameTransport, opts Options, events chan protocol.Event) *session {
	s := &session{
		transport: transport,
		opts:      opts,
		events:    events,
		pending:   make(map[string]chan *protocol.Response),
		done:      make(chan struct{}),
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
			s.emit(protocol.Event{Topic: frame.Topic, Data: frame.Event})
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
