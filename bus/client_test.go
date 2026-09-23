package bus

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/siyouyun-open/siyouyun_sdk/v2/protocol"
)

// fakeBus is a minimal bus server (UDS + JSON-lines) used to verify the client's
// request shape, response decoding, event dispatch, and reconnect replay.
type fakeBus struct {
	t        *testing.T
	path     string
	listener net.Listener

	mu       sync.Mutex
	conns    []net.Conn
	requests []protocol.Request
	callFn   func(protocol.Request) protocol.Response

	done chan struct{}
}

func newFakeBus(t *testing.T) *fakeBus {
	t.Helper()
	// UDS paths are length-limited (about 104 bytes on macOS); t.TempDir() is too long, so use a short name under /tmp.
	path := filepath.Join(os.TempDir(), fmt.Sprintf("appsdk-bus-%d-%d.sock", os.Getpid(), time.Now().UnixNano()))
	_ = os.Remove(path)
	listener, err := net.Listen("unix", path)
	if err != nil {
		t.Fatalf("listen unix: %v", err)
	}

	f := &fakeBus{t: t, path: path, listener: listener, done: make(chan struct{})}
	go f.acceptLoop()
	t.Cleanup(f.Close)
	return f
}

func (f *fakeBus) acceptLoop() {
	for {
		conn, err := f.listener.Accept()
		if err != nil {
			return
		}
		f.mu.Lock()
		f.conns = append(f.conns, conn)
		f.mu.Unlock()
		go f.serve(conn)
	}
}

func (f *fakeBus) serve(conn net.Conn) {
	scanner := bufio.NewScanner(conn)
	scanner.Buffer(make([]byte, 64<<10), 1<<20)
	for scanner.Scan() {
		var req protocol.Request
		if err := json.Unmarshal(scanner.Bytes(), &req); err != nil {
			continue
		}
		f.mu.Lock()
		f.requests = append(f.requests, req)
		f.mu.Unlock()

		switch req.Type {
		case protocol.TypePing:
			f.write(conn, protocol.Response{Type: protocol.FramePong, ID: req.ID})
		case protocol.TypeSubscribe, protocol.TypeUnsubscribe:
			f.write(conn, protocol.Response{Type: protocol.FrameResponse, ID: req.ID, Code: protocol.CodeSuccess})
		case protocol.TypeCall:
			resp := protocol.Response{Type: protocol.FrameResponse, ID: req.ID, Code: protocol.CodeSuccess}
			if f.callFn != nil {
				resp = f.callFn(req)
				resp.Type = protocol.FrameResponse
				resp.ID = req.ID
			}
			f.write(conn, resp)
		}
	}
}

func (f *fakeBus) write(conn net.Conn, resp protocol.Response) {
	raw, err := json.Marshal(resp)
	if err != nil {
		return
	}
	_, _ = conn.Write(append(raw, '\n'))
}

// PushEvent pushes one event to all connected clients.
func (f *fakeBus) PushEvent(topic string, payload any) {
	raw, _ := json.Marshal(payload)
	resp := protocol.Response{Type: protocol.FrameEvent, ID: "evt", Topic: topic, Event: raw}
	f.mu.Lock()
	conns := append([]net.Conn(nil), f.conns...)
	f.mu.Unlock()
	for _, conn := range conns {
		f.write(conn, resp)
	}
}

// DropConns drops all current connections, used to verify client reconnect.
func (f *fakeBus) DropConns() {
	f.mu.Lock()
	conns := append([]net.Conn(nil), f.conns...)
	f.conns = nil
	f.mu.Unlock()
	for _, conn := range conns {
		_ = conn.Close()
	}
}

func (f *fakeBus) Requests() []protocol.Request {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]protocol.Request(nil), f.requests...)
}

// WaitRequests waits for at least n requests to arrive.
func (f *fakeBus) WaitRequests(n int, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if len(f.Requests()) >= n {
			return true
		}
		time.Sleep(5 * time.Millisecond)
	}
	return false
}

func (f *fakeBus) Close() {
	select {
	case <-f.done:
		return
	default:
	}
	close(f.done)
	_ = f.listener.Close()
	f.DropConns()
	_ = os.Remove(f.path)
}

func testOptions(f *fakeBus) Options {
	return Options{
		SocketPath:  f.path,
		CallTimeout: 3 * time.Second,
		DialTimeout: time.Second,
		MinBackoff:  20 * time.Millisecond,
		MaxBackoff:  100 * time.Millisecond,
	}
}

func TestCallReturnsResponse(t *testing.T) {
	fake := newFakeBus(t)
	fake.callFn = func(req protocol.Request) protocol.Response {
		if req.Template != protocol.TemplateKVGet {
			return protocol.Response{Code: protocol.CodeError, Error: "unexpected template " + req.Template}
		}
		return protocol.Response{Code: protocol.CodeSuccess, Data: json.RawMessage(`{"ok":true}`)}
	}

	client := New(testOptions(fake))
	defer client.Close()

	var out struct {
		OK bool `json:"ok"`
	}
	if err := client.CallTemplateInto(context.Background(), protocol.TemplateKVGet, map[string]string{"type": "t", "key": "k"}, &out); err != nil {
		t.Fatalf("CallTemplateInto: %v", err)
	}
	if !out.OK {
		t.Fatalf("unexpected decoded result: %#v", out)
	}
	if state := client.State(); !state.Connected || state.Transport != "unix" {
		t.Fatalf("unexpected state: %#v", state)
	}

	reqs := fake.Requests()
	if len(reqs) != 1 || reqs[0].Template != protocol.TemplateKVGet {
		t.Fatalf("unexpected requests: %#v", reqs)
	}
	// Identity fields must not be self-reported by the client.
	var payload map[string]any
	_ = json.Unmarshal(reqs[0].Payload, &payload)
	for _, forbidden := range []string{"username", "namespace", "appCode"} {
		if _, ok := payload[forbidden]; ok {
			t.Fatalf("client must not send identity field %q: %#v", forbidden, payload)
		}
	}
}

func TestCallErrorResponseBecomesCallError(t *testing.T) {
	fake := newFakeBus(t)
	fake.callFn = func(protocol.Request) protocol.Response {
		return protocol.Response{Code: protocol.CodeError, Error: "key is required"}
	}
	client := New(testOptions(fake))
	defer client.Close()

	err := client.CallTemplateInto(context.Background(), protocol.TemplateKVGet, nil, nil)
	var callErr *CallError
	if err == nil {
		t.Fatal("expected error")
	}
	if !errors.As(err, &callErr) || callErr.Code != protocol.CodeError {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestSubscribeReceivesEvents(t *testing.T) {
	fake := newFakeBus(t)
	fake.callFn = func(protocol.Request) protocol.Response {
		return protocol.Response{Code: protocol.CodeSuccess}
	}
	client := New(testOptions(fake))
	defer client.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := client.Subscribe(ctx, protocol.TopicFileEvent); err != nil {
		t.Fatalf("Subscribe: %v", err)
	}

	fake.PushEvent(protocol.TopicFileEvent+".com.example.app", map[string]any{"action": 1, "ufi": "/ufi/pool-raw/syspool/a.txt"})

	select {
	case event := <-client.Events():
		if event.Topic != protocol.TopicFileEvent+".com.example.app" {
			t.Fatalf("unexpected topic %q", event.Topic)
		}
		var body struct {
			Action int    `json:"action"`
			UFI    string `json:"ufi"`
		}
		if err := json.Unmarshal(event.Data, &body); err != nil {
			t.Fatalf("decode event: %v", err)
		}
		if body.Action != 1 || body.UFI == "" {
			t.Fatalf("unexpected event body: %#v", body)
		}
	case <-ctx.Done():
		t.Fatal("timed out waiting for event")
	}
}

// After the connection drops the client must reconnect automatically and idempotently replay all subscribed topics.
func TestReconnectReplaysSubscriptions(t *testing.T) {
	fake := newFakeBus(t)
	fake.callFn = func(protocol.Request) protocol.Response {
		return protocol.Response{Code: protocol.CodeSuccess}
	}
	client := New(testOptions(fake))
	defer client.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := client.Subscribe(ctx, protocol.TopicNotificationEvent); err != nil {
		t.Fatalf("Subscribe: %v", err)
	}

	fake.DropConns()

	deadline := time.Now().Add(3 * time.Second)
	subscribes := 0
	for time.Now().Before(deadline) {
		subscribes = 0
		for _, req := range fake.Requests() {
			if req.Type == protocol.TypeSubscribe {
				subscribes++
			}
		}
		if subscribes >= 2 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if subscribes < 2 {
		t.Fatalf("subscribe was not replayed after reconnect (saw %d)", subscribes)
	}
}

// When there is no UDS and only HTTP, Call still works (HTTP capability channel fallback).
func TestHTTPCallFallback(t *testing.T) {
	server := newFakeHTTPBus(t, func(template string) (int, string) {
		if template != protocol.TemplateDaemonHealth {
			t.Fatalf("unexpected template %q", template)
		}
		return 200, `{"success":true,"result":{"online":true}}`
	})

	client := New(Options{HTTPURL: server, CallerToken: "tok", CallTimeout: 3 * time.Second})
	defer client.Close()

	var out struct {
		Online bool `json:"online"`
	}
	if err := client.CallTemplateInto(context.Background(), protocol.TemplateDaemonHealth, nil, &out); err != nil {
		t.Fatalf("CallTemplateInto: %v", err)
	}
	if !out.Online {
		t.Fatalf("unexpected result: %#v", out)
	}
	if state := client.State(); state.Transport != "http" {
		t.Fatalf("unexpected state: %#v", state)
	}
}

// The first reconnect failure must also keep retrying.
//
// Real-device root cause (2026-09-22): supervise's retry loop used `c.sess != sess` to decide
// "whether the connection has already been replaced", but it set c.sess to nil before the first
// attempt — so **a single failure gave up permanently**.
// App-side symptom: after a daemon restart the log held only one "reconnect failed" line, then
// nothing more; events were never delivered again (recovery required restarting the app process).
func TestClientKeepsRetryingAfterFailedReconnect(t *testing.T) {
	path := filepath.Join(os.TempDir(), fmt.Sprintf("appsdk-bus-retry-%d-%d.sock", os.Getpid(), time.Now().UnixNano()))
	_ = os.Remove(path)
	listener, err := net.Listen("unix", path)
	if err != nil {
		t.Fatalf("listen unix: %v", err)
	}
	first := &fakeBus{t: t, path: path, listener: listener, done: make(chan struct{})}
	go first.acceptLoop()

	client := New(Options{
		SocketPath:  path,
		WSURL:       "",
		MinBackoff:  5 * time.Millisecond,
		MaxBackoff:  20 * time.Millisecond,
		DialTimeout: 200 * time.Millisecond,
	})
	defer client.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := client.Subscribe(ctx, protocol.TopicFileEvent); err != nil {
		t.Fatalf("Subscribe: %v", err)
	}

	// Close the server and client connections, and leave "the server absent" for a
	// short while afterwards, guaranteeing at least one failed reconnect attempt.
	first.mu.Lock()
	conns := append([]net.Conn(nil), first.conns...)
	first.mu.Unlock()
	for _, c := range conns {
		_ = c.Close()
	}
	_ = first.listener.Close()
	_ = os.Remove(path)
	time.Sleep(60 * time.Millisecond) // Several multiples of MinBackoff: a failure must already have occurred

	// The server is back (recreated on the same path).
	listener2, err := net.Listen("unix", path)
	if err != nil {
		t.Fatalf("listen unix(2): %v", err)
	}
	second := &fakeBus{t: t, path: path, listener: listener2, done: make(chan struct{})}
	go second.acceptLoop()
	defer func() { _ = listener2.Close() }()

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		second.mu.Lock()
		reqs := append([]protocol.Request(nil), second.requests...)
		second.mu.Unlock()
		for _, req := range reqs {
			if req.Type == protocol.TypeSubscribe {
				return // reconnect succeeded and the subscriptions were replayed
			}
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("no retry after the first failed reconnect: the client gave up permanently (on a real device this shows up as events that never arrive again)")
}

// After a daemon restart (process gone, socket recreated), the client must be able to
// re-establish the connection itself **and replay its subscriptions**.
//
// On a real device this behavior is one of the root causes of "events silently stop being
// delivered": the subscription registry is persisted, the connection is not.
// This unit test pins it down directly, so it does not have to be debugged on a real device.
func TestClientReconnectsAndReplaysSubscriptions(t *testing.T) {
	path := filepath.Join(os.TempDir(), fmt.Sprintf("appsdk-bus-reconnect-%d-%d.sock", os.Getpid(), time.Now().UnixNano()))
	_ = os.Remove(path)

	start := func() (*fakeBus, chan struct{}) {
		listener, err := net.Listen("unix", path)
		if err != nil {
			t.Fatalf("listen unix: %v", err)
		}
		f := &fakeBus{t: t, path: path, listener: listener, done: make(chan struct{})}
		go f.acceptLoop()
		return f, f.done
	}

	first, _ := start()
	// Record the subscribe frames received on the first connection, to compare against the replay after reconnect later.
	resubscribed := make(chan []string, 4)
	first.callFn = nil

	client := New(Options{
		SocketPath:  path,
		WSURL:       "", // UDS only: attribute "reconnect succeeded" to the UDS being recreated
		MinBackoff:  10 * time.Millisecond,
		MaxBackoff:  50 * time.Millisecond,
		DialTimeout: 500 * time.Millisecond,
	})
	defer client.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := client.Subscribe(ctx, protocol.TopicFileEvent); err != nil {
		t.Fatalf("Subscribe: %v", err)
	}

	// Simulate a daemon restart: close the old listener and connections, delete the socket, then recreate it at the same path.
	first.mu.Lock()
	conns := append([]net.Conn(nil), first.conns...)
	first.mu.Unlock()
	for _, c := range conns {
		_ = c.Close()
	}
	_ = first.listener.Close()
	_ = os.Remove(path)

	second, _ := start()
	// Collect the subscribe frames of the second generation.
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		second.mu.Lock()
		reqs := append([]protocol.Request(nil), second.requests...)
		second.mu.Unlock()
		for _, req := range reqs {
			if req.Type == protocol.TypeSubscribe {
				select {
				case resubscribed <- req.Topics:
				default:
				}
			}
		}
		if len(resubscribed) > 0 {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}

	select {
	case topics := <-resubscribed:
		found := false
		for _, topic := range topics {
			if topic == protocol.TopicFileEvent {
				found = true
			}
		}
		if !found {
			t.Fatalf("replayed subscriptions after reconnect = %v, want it to contain %s", topics, protocol.TopicFileEvent)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("subscriptions were not replayed after reconnect: events are silently never delivered again after a daemon restart")
	}
}
