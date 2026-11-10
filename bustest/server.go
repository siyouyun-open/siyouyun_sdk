// Package bustest provides a "fake daemon": a minimal WebSocket bus service that implements
// the replies for subscribe / unsubscribe / call / ack and pushes events.
//
// Why it is needed: the new SDK has no injectable bus for applications to use
// (`capability.New` requires a real `*bus.Client`, and the SDK's own fakebus is not exported),
// so an application **cannot test its own event handling path end to end** — it can only call
// the handler directly, which means the whole "is the subscription actually wired up, does the
// event decode correctly, are the identity and space right" segment is never tested. On a real
// device this shows up as "the code looks right, but the events just never arrive".
//
// Usage (application-side tests):
//
//	srv := bustest.New(t, bustest.Options{AppID: "album"})
//	app := appsdk.NewWithOptions(rt, appsdk.WithBusWS(srv.URL()))
//	sub, err := capability.SubscribeFileEvents(ctx, app.Bus(), handler)
//	srv.WaitForSubscription(t, protocol.TopicFileEvent)   // wait until the subscription is really registered
//	srv.Publish(t, protocol.TopicFileEvent, producerPayload)
//	// assert the handler wrote to the DB / generated a thumbnail / rejected an out-of-scope space
//
// It does two things the daemon's semantics require: topic **expansion** (`system.file.event`
// → `system.file.event.<AppID>`, see minictl's scopeBusTopic) and the subscription ack.
//
// Reliable (at-least-once) subscriptions are supported as well: PublishReliable assigns an
// event id, keeps the delivery unacknowledged until an ACK frame arrives for it, and
// redelivers it after Options.AckWait — including on a new connection after a disconnect.
// Use PublishReliable for reliable subscriptions and plain Publish for fire-and-forget ones;
// the two never cross-deliver.
package bustest

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"

	"github.com/siyouyun-open/siyouyun_sdk/v2/protocol"
)

// defaultAckWait is the fake daemon's redelivery timeout when Options.AckWait is unset.
// It is short on purpose: the point of the fake is to make the redelivery loop observable in a
// unit test, not to model the daemon's 30s production value.
const defaultAckWait = 300 * time.Millisecond

// Options configures the fake daemon.
type Options struct {
	// AppID expands `system.file.event` into `system.file.event.<AppID>`, matching the daemon's
	// scopeBusTopic. When it is empty, the publish side and the subscribe side both match the
	// topic as is.
	AppID string
	// CallCode / CallData are the default reply to a call request. By default it succeeds with
	// empty data.
	CallCode string
	CallData json.RawMessage
	// OnCall customizes the reply per template (an empty code means use the default).
	//
	// Why it is needed: an application often calls several templates in one test (for example,
	// read the watch scope, then change it, then read it back), which a single CallData cannot
	// express. The returned data is placed into the reply frame as is.
	OnCall func(req protocol.Request) (code string, data json.RawMessage)
	// AckWait is how long an unacknowledged reliable delivery waits before it is pushed again
	// (the fake counterpart of the daemon's AckWait). Defaults to 300ms.
	AckWait time.Duration
}

// Server is a fake daemon instance.
type Server struct {
	opts Options
	ts   *httptest.Server

	mu       sync.Mutex
	conns    map[*serverConn]struct{}
	calls    int
	requests []protocol.Request

	// Reliable-delivery bookkeeping, keyed by the daemon-assigned event id (a decimal
	// sequence number, like the JetStream stream sequence it stands for).
	nextEventID uint64
	pending     map[string]*pendingDelivery
	deliveries  map[string]int
	acked       map[string]struct{}
	ackRequests []string
}

// pendingDelivery is one reliable event that has not been acknowledged yet.
type pendingDelivery struct {
	topic string // expanded topic
	event json.RawMessage
	timer *time.Timer
}

type serverConn struct {
	ws      *websocket.Conn
	writeMu sync.Mutex
	// topics maps the expanded topic to whether the subscription asked for reliable
	// (at-least-once) delivery.
	topics map[string]bool
}

// New starts the fake daemon; it is closed automatically when the test ends.
func New(t *testing.T, opts Options) *Server {
	t.Helper()
	if opts.AckWait <= 0 {
		opts.AckWait = defaultAckWait
	}
	s := &Server{
		opts:       opts,
		conns:      map[*serverConn]struct{}{},
		pending:    map[string]*pendingDelivery{},
		deliveries: map[string]int{},
		acked:      map[string]struct{}{},
	}

	upgrader := websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}
	mux := http.NewServeMux()
	mux.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) {
		ws, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		sc := &serverConn{ws: ws, topics: map[string]bool{}}
		s.mu.Lock()
		s.conns[sc] = struct{}{}
		s.mu.Unlock()
		go s.serve(sc)
	})

	s.ts = httptest.NewServer(mux)
	t.Cleanup(func() {
		s.mu.Lock()
		for _, p := range s.pending {
			if p.timer != nil {
				p.timer.Stop()
			}
		}
		for sc := range s.conns {
			_ = sc.ws.Close()
		}
		s.mu.Unlock()
		s.ts.Close()
	})
	return s
}

// URL returns the address that can be passed to `appsdk.WithBusWS`.
func (s *Server) URL() string {
	return "ws" + strings.TrimPrefix(s.ts.URL, "http") + "/ws"
}

// expand replicates the daemon's scopeBusTopic: it expands a system topic template into the
// caller's own topic.
func (s *Server) expand(topic string) string {
	if topic == protocol.TopicFileEvent && s.opts.AppID != "" {
		return protocol.TopicFileEvent + "." + s.opts.AppID
	}
	return topic
}

func (s *Server) serve(sc *serverConn) {
	defer func() {
		s.mu.Lock()
		delete(s.conns, sc)
		s.mu.Unlock()
		_ = sc.ws.Close()
	}()

	for {
		_, raw, err := sc.ws.ReadMessage()
		if err != nil {
			return
		}
		var req protocol.Request
		if err := json.Unmarshal(raw, &req); err != nil {
			continue
		}
		switch req.Type {
		case protocol.TypeSubscribe:
			sc.writeMu.Lock()
			for _, topic := range req.Topics {
				sc.topics[s.expand(topic)] = req.Reliable
			}
			sc.writeMu.Unlock()
			// Same shape as the daemon: plain subscriptions confirm with `{"subscribed":true}`,
			// reliable ones additionally confirm `"reliable":true` (the SDK refuses to treat a
			// silent confirmation as at-least-once).
			confirmation := json.RawMessage(`{"subscribed":true}`)
			if req.Reliable {
				confirmation = json.RawMessage(`{"subscribed":true,"reliable":true}`)
			}
			s.reply(sc, req.ID, codeOK, confirmation)
			if req.Reliable {
				// Reconnect hint: the durable consumer still holds every unacknowledged
				// delivery of these topics, so replay them on the new connection.
				s.resendPending(req.Topics)
			}
		case protocol.TypeUnsubscribe:
			sc.writeMu.Lock()
			for _, topic := range req.Topics {
				delete(sc.topics, s.expand(topic))
			}
			sc.writeMu.Unlock()
			s.reply(sc, req.ID, codeOK, nil)
		case protocol.TypeCall:
			s.mu.Lock()
			s.calls++
			s.requests = append(s.requests, req)
			s.mu.Unlock()
			code := s.opts.CallCode
			data := s.opts.CallData
			if s.opts.OnCall != nil {
				if customCode, customData := s.opts.OnCall(req); customCode != "" || customData != nil {
					if customCode != "" {
						code = customCode
					}
					if customData != nil {
						data = customData
					}
				}
			}
			if code == "" {
				code = codeOK
			}
			s.reply(sc, req.ID, code, data)
		case protocol.TypeAck:
			s.handleAck(req.EventID)
		case protocol.TypePing:
			sc.send(protocol.Response{Type: protocol.FramePong, ID: req.ID})
		}
	}
}

const codeOK = "A00200"

func (s *Server) reply(sc *serverConn, id, code string, data json.RawMessage) {
	sc.send(protocol.Response{Type: protocol.FrameResponse, ID: id, Code: code, Data: data})
}

func (sc *serverConn) send(frame protocol.Response) {
	raw, err := json.Marshal(frame)
	if err != nil {
		return
	}
	sc.writeMu.Lock()
	defer sc.writeMu.Unlock()
	_ = sc.ws.WriteMessage(websocket.TextMessage, raw)
}

// WaitForSubscription waits until a (non-expanded) topic has been subscribed to.
//
// Waiting is mandatory: the subscription is an asynchronous round trip, and an immediately
// following Publish occasionally delivers an event before the subscription arrives, which
// looks like "the event was not handled". The same race exists on a real device; it is just
// masked by a longer startup time.
func (s *Server) WaitForSubscription(t *testing.T, topic string) {
	t.Helper()
	want := s.expand(topic)
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		s.mu.Lock()
		for sc := range s.conns {
			sc.writeMu.Lock()
			_, ok := sc.topics[want]
			sc.writeMu.Unlock()
			if ok {
				s.mu.Unlock()
				return
			}
		}
		s.mu.Unlock()
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for subscription %s", want)
}

// Publish pushes one fire-and-forget event to every connection subscribed to the topic
// (reliable subscriptions are skipped: their deliveries must be tracked, use PublishReliable)
// and returns the number of connections it was delivered to.
//
// event uses the **producer's real shape** (for example the platform file event's
// `{ugn, srcUfi, ufi, inode, action, withAvatar}`) so that the decode path is actually
// covered.
func (s *Server) Publish(t *testing.T, topic string, event any) int {
	t.Helper()
	raw, err := json.Marshal(event)
	if err != nil {
		t.Fatalf("marshal event: %v", err)
	}
	return s.deliver(s.expand(topic), raw, false)
}

// PublishReliable pushes one at-least-once event to every reliable subscription of the topic
// and returns its daemon-assigned event id (a decimal sequence number, the stand-in for a
// JetStream stream sequence).
//
// The delivery is kept pending until an ACK frame for that id arrives; after Options.AckWait
// it is pushed again (and that repeats until it is acknowledged). Pending deliveries are also
// re-pushed when a reliable subscription is (re)established, which is what the real daemon
// does after a reconnect: the durable consumer still holds everything that was not acked.
func (s *Server) PublishReliable(t *testing.T, topic string, event any) string {
	t.Helper()
	raw, err := json.Marshal(event)
	if err != nil {
		t.Fatalf("marshal event: %v", err)
	}
	expanded := s.expand(topic)

	s.mu.Lock()
	s.nextEventID++
	id := strconv.FormatUint(s.nextEventID, 10)
	s.pending[id] = &pendingDelivery{topic: expanded, event: raw}
	s.mu.Unlock()

	s.deliverPending(id)
	s.scheduleRedelivery(id)
	return id
}

// deliver sends an event frame without a delivery id; reliableOnly=false delivers to
// fire-and-forget subscriptions, reliableOnly=true to reliable ones.
func (s *Server) deliver(expanded string, raw json.RawMessage, reliableOnly bool) int {
	frame := protocol.Response{
		Type:  protocol.FrameEvent,
		ID:    time.Now().UTC().Format(time.RFC3339Nano),
		Topic: expanded,
		Event: raw,
	}

	s.mu.Lock()
	targets := make([]*serverConn, 0, len(s.conns))
	for sc := range s.conns {
		sc.writeMu.Lock()
		reliable, ok := sc.topics[expanded]
		sc.writeMu.Unlock()
		if !ok || reliable != reliableOnly {
			continue
		}
		targets = append(targets, sc)
	}
	s.mu.Unlock()

	for _, sc := range targets {
		sc.send(frame)
	}
	return len(targets)
}

// deliverPending (re)pushes one tracked reliable delivery to the reliable subscriptions of
// its topic and counts the attempt.
func (s *Server) deliverPending(id string) int {
	s.mu.Lock()
	p := s.pending[id]
	if p == nil {
		s.mu.Unlock()
		return 0
	}
	frame := protocol.Response{
		Type:    protocol.FrameEvent,
		ID:      time.Now().UTC().Format(time.RFC3339Nano),
		Topic:   p.topic,
		Event:   p.event,
		EventID: id,
	}
	targets := make([]*serverConn, 0, len(s.conns))
	for sc := range s.conns {
		sc.writeMu.Lock()
		reliable := sc.topics[p.topic]
		sc.writeMu.Unlock()
		if reliable {
			targets = append(targets, sc)
		}
	}
	s.deliveries[id] += len(targets)
	s.mu.Unlock()

	for _, sc := range targets {
		sc.send(frame)
	}
	return len(targets)
}

// scheduleRedelivery arms (or re-arms) the AckWait timer of a pending delivery.
func (s *Server) scheduleRedelivery(id string) {
	s.mu.Lock()
	p := s.pending[id]
	if p == nil {
		s.mu.Unlock()
		return
	}
	if p.timer != nil {
		p.timer.Stop()
	}
	p.timer = time.AfterFunc(s.opts.AckWait, func() {
		s.mu.Lock()
		stillPending := s.pending[id] != nil
		s.mu.Unlock()
		if !stillPending {
			return
		}
		s.deliverPending(id)
		s.scheduleRedelivery(id)
	})
	s.mu.Unlock()
}

// handleAck commits one reliable delivery; an ACK for an unknown or already-acked id is
// ignored (a late ACK from before a reconnect is expected on the wire).
func (s *Server) handleAck(eventID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ackRequests = append(s.ackRequests, eventID)
	if p, ok := s.pending[eventID]; ok {
		if p.timer != nil {
			p.timer.Stop()
		}
		delete(s.pending, eventID)
		s.acked[eventID] = struct{}{}
	}
}

// resendPending re-pushes every pending delivery of the given (non-expanded) topics. It runs
// after a reliable subscription has been confirmed, which also covers the reconnect case.
func (s *Server) resendPending(topics []string) {
	expanded := make(map[string]struct{}, len(topics))
	for _, topic := range topics {
		expanded[s.expand(topic)] = struct{}{}
	}
	s.mu.Lock()
	ids := make([]string, 0, len(s.pending))
	for id, p := range s.pending {
		if _, ok := expanded[p.topic]; ok {
			ids = append(ids, id)
		}
	}
	s.mu.Unlock()

	sort.Slice(ids, func(i, j int) bool {
		a, _ := strconv.ParseUint(ids[i], 10, 64)
		b, _ := strconv.ParseUint(ids[j], 10, 64)
		return a < b
	})
	for _, id := range ids {
		s.deliverPending(id)
	}
}

// WaitForAck waits until the delivery with this event id has been acknowledged.
func (s *Server) WaitForAck(t *testing.T, eventID string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if s.IsAcked(eventID) {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for ack of event %s (pending=%d, deliveries=%d)",
		eventID, s.PendingCount(), s.Deliveries(eventID))
}

// WaitForDeliveries waits until the delivery with this event id has been pushed at least want
// times.
func (s *Server) WaitForDeliveries(t *testing.T, eventID string, want int) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if s.Deliveries(eventID) >= want {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %d deliveries of event %s (got %d)", want, eventID, s.Deliveries(eventID))
}

// Deliveries returns how many times the event was pushed to subscribed connections.
func (s *Server) Deliveries(eventID string) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.deliveries[eventID]
}

// IsAcked reports whether an ACK for the event was received.
func (s *Server) IsAcked(eventID string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, ok := s.acked[eventID]
	return ok
}

// PendingCount returns the number of reliable deliveries that are still waiting for an ACK.
func (s *Server) PendingCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.pending)
}

// AckCount returns the number of ACK frames received (including ACKs for unknown or
// already-acked ids).
func (s *Server) AckCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.ackRequests)
}

// AckedEventIDs returns the event ids that were acknowledged, in ACK arrival order.
func (s *Server) AckedEventIDs() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.ackRequests...)
}

// DropConnections closes all live connections, simulating a daemon restart / network drop.
// Pending reliable deliveries are kept, so a reconnecting reliable client gets them re-pushed.
func (s *Server) DropConnections() {
	s.mu.Lock()
	conns := make([]*serverConn, 0, len(s.conns))
	for sc := range s.conns {
		conns = append(conns, sc)
	}
	s.mu.Unlock()
	for _, sc := range conns {
		_ = sc.ws.Close()
	}
}

// CallCount returns the number of call requests received (used to assert behavior such as
// "no outgoing call when parameter validation fails").
func (s *Server) CallCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.calls
}

// Requests returns all call requests received (in arrival order).
func (s *Server) Requests() []protocol.Request {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]protocol.Request(nil), s.requests...)
}

// LastCall returns the request body (decoded payload) of the most recent call to a template,
// or ok=false if it was never called.
func (s *Server) LastCall(template string) (map[string]any, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := len(s.requests) - 1; i >= 0; i-- {
		if string(s.requests[i].Template) != template {
			continue
		}
		var payload map[string]any
		if len(s.requests[i].Payload) > 0 {
			_ = json.Unmarshal(s.requests[i].Payload, &payload)
		}
		return payload, true
	}
	return nil, false
}
