// Package bustest provides a "fake daemon": a minimal WebSocket bus service that implements
// the replies for subscribe / unsubscribe / call and pushes events.
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
package bustest

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"

	"github.com/siyouyun-open/siyouyun_sdk/v2/protocol"
)

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
}

// Server is a fake daemon instance.
type Server struct {
	opts Options
	ts   *httptest.Server

	mu       sync.Mutex
	conns    map[*serverConn]struct{}
	calls    int
	requests []protocol.Request
}

type serverConn struct {
	ws      *websocket.Conn
	writeMu sync.Mutex
	topics  map[string]struct{}
}

// New starts the fake daemon; it is closed automatically when the test ends.
func New(t *testing.T, opts Options) *Server {
	t.Helper()
	s := &Server{opts: opts, conns: map[*serverConn]struct{}{}}

	upgrader := websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}
	mux := http.NewServeMux()
	mux.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) {
		ws, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		sc := &serverConn{ws: ws, topics: map[string]struct{}{}}
		s.mu.Lock()
		s.conns[sc] = struct{}{}
		s.mu.Unlock()
		go s.serve(sc)
	})

	s.ts = httptest.NewServer(mux)
	t.Cleanup(func() {
		s.mu.Lock()
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
				sc.topics[s.expand(topic)] = struct{}{}
			}
			sc.writeMu.Unlock()
			s.ack(sc, req.ID, codeOK, nil)
		case protocol.TypeUnsubscribe:
			sc.writeMu.Lock()
			for _, topic := range req.Topics {
				delete(sc.topics, s.expand(topic))
			}
			sc.writeMu.Unlock()
			s.ack(sc, req.ID, codeOK, nil)
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
			s.ack(sc, req.ID, code, data)
		case protocol.TypePing:
			sc.send(protocol.Response{Type: protocol.FramePong, ID: req.ID})
		}
	}
}

const codeOK = "A00200"

func (s *Server) ack(sc *serverConn, id, code string, data json.RawMessage) {
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

// Publish pushes one event to every connection subscribed to the topic and returns the number
// of connections it was delivered to.
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
	frame := protocol.Response{
		Type:  protocol.FrameEvent,
		ID:    time.Now().UTC().Format(time.RFC3339Nano),
		Topic: s.expand(topic),
		Event: raw,
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	delivered := 0
	for sc := range s.conns {
		sc.writeMu.Lock()
		_, ok := sc.topics[s.expand(topic)]
		sc.writeMu.Unlock()
		if !ok {
			continue
		}
		sc.send(frame)
		delivered++
	}
	return delivered
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
