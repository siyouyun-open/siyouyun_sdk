package capability

import (
	"context"
	"encoding/json"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/nats-io/nats-server/v2/server"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"

	"github.com/siyouyun-open/siyouyun_sdk/v2/bus"
	"github.com/siyouyun-open/siyouyun_sdk/v2/bustest"
	"github.com/siyouyun-open/siyouyun_sdk/v2/protocol"
)

func startTestNATS(t *testing.T) (string, func()) {
	t.Helper()
	opts := &server.Options{
		Host: "127.0.0.1", Port: -1,
		JetStream: true, StoreDir: t.TempDir(),
		NoLog: true, NoSigs: true,
	}
	srv, err := server.NewServer(opts)
	if err != nil {
		t.Fatal(err)
	}
	go srv.Start()
	if !srv.ReadyForConnections(5 * time.Second) {
		t.Fatal("embedded nats not ready")
	}
	return srv.ClientURL(), func() { srv.Shutdown() }
}

func TestStartFileEventListenerEndToEnd(t *testing.T) {
	natsURL, shutdown := startTestNATS(t)
	defer shutdown()

	nc, err := nats.Connect(natsURL)
	if err != nil {
		t.Fatal(err)
	}
	defer nc.Close()
	js, err := jetstream.New(nc)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	stream, err := js.CreateOrUpdateStream(ctx, jetstream.StreamConfig{
		Name:      "SYY_INTEREST_SHARED_STREAM",
		Subjects:  []string{"os.fs.event"},
		Retention: jetstream.InterestPolicy,
		Storage:   jetstream.FileStorage,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := stream.CreateOrUpdateConsumer(ctx, jetstream.ConsumerConfig{
		Durable:       "interest_lst-test",
		FilterSubject: "os.fs.event",
		AckPolicy:     jetstream.AckExplicitPolicy,
		DeliverPolicy: jetstream.DeliverNewPolicy,
	}); err != nil {
		t.Fatal(err)
	}

	const listenerID = "lst-test"
	srv := bustest.New(t, bustest.Options{
		OnCall: func(req protocol.Request) (string, json.RawMessage) {
			switch string(req.Template) {
			case protocol.TemplateInterestListen:
				data, _ := json.Marshal(interestListenResponse{
					ListenerID: listenerID, Consumer: "interest_lst-test",
					Stream: "SYY_INTEREST_SHARED_STREAM", Subject: "os.fs.event",
					NATSURL: natsURL,
				})
				return protocol.CodeSuccess, data
			case protocol.TemplateInterestStop, protocol.TemplateInterestRenew:
				data, _ := json.Marshal(map[string]any{"ok": true})
				return protocol.CodeSuccess, data
			default:
				return "", nil
			}
		},
	})
	client := bus.New(bus.Options{WSURL: srv.URL()})
	defer client.Close()

	got := make(chan FileEvent, 1)
	listener, err := StartFileEventListener(ctx, client, FileEventListenerOptions{
		Dirs: []string{"/Photos"}, NATSURL: natsURL,
	}, func(ev FileEvent) error {
		got <- ev
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	time.Sleep(300 * time.Millisecond) // let Fetch start

	payload := []byte(`{"v":2,"ufi":"/ufi/pool-raw/syspool/Photos/a.jpg","action":1,"inode":1,"ugn":{"username":"alice","groupName":"","namespace":"main"},"matched":{"apps":["album"],"listeners":["lst-test"]}}`)
	if _, err := js.Publish(ctx, "os.fs.event", payload); err != nil {
		t.Fatal(err)
	}
	select {
	case ev := <-got:
		if ev.UFI != "/ufi/pool-raw/syspool/Photos/a.jpg" || ev.Action != 1 {
			t.Fatalf("unexpected event: %+v", ev)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("listener did not receive the event")
	}
}

func TestFileEventListenerRecoversAfterRenewFailure(t *testing.T) {
	natsURL, shutdown := startTestNATS(t)
	defer shutdown()
	nc, err := nats.Connect(natsURL)
	if err != nil {
		t.Fatal(err)
	}
	defer nc.Close()
	js, err := jetstream.New(nc)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	stream, err := js.CreateOrUpdateStream(ctx, jetstream.StreamConfig{
		Name: "SYY_INTEREST_SHARED_STREAM", Subjects: []string{"os.fs.event"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := stream.CreateOrUpdateConsumer(ctx, jetstream.ConsumerConfig{
		Durable: "interest_lst-recover", FilterSubject: "os.fs.event",
		AckPolicy: jetstream.AckExplicitPolicy, DeliverPolicy: jetstream.DeliverNewPolicy,
	}); err != nil {
		t.Fatal(err)
	}
	var listens atomic.Int32
	srv := bustest.New(t, bustest.Options{
		OnCall: func(req protocol.Request) (string, json.RawMessage) {
			switch string(req.Template) {
			case protocol.TemplateInterestListen:
				n := listens.Add(1)
				data, _ := json.Marshal(interestListenResponse{
					ListenerID: fmt.Sprintf("lst-%d", n), Consumer: "interest_lst-recover",
					Stream: "SYY_INTEREST_SHARED_STREAM", Subject: "os.fs.event", NATSURL: natsURL,
				})
				return protocol.CodeSuccess, data
			case protocol.TemplateInterestRenew:
				return protocol.CodeError, nil
			case protocol.TemplateInterestStop:
				return protocol.CodeSuccess, json.RawMessage(`{}`)
			}
			return "", nil
		},
	})
	client := bus.New(bus.Options{WSURL: srv.URL()})
	defer client.Close()
	listener, err := StartFileEventListener(context.Background(), client, FileEventListenerOptions{
		Dirs: []string{"/Photos"}, NATSURL: natsURL, TTL: 300 * time.Millisecond,
	}, func(FileEvent) error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) && listens.Load() < 2 {
		time.Sleep(50 * time.Millisecond)
	}
	if got := listens.Load(); got < 2 {
		t.Fatalf("renew failure should trigger a new listen, got %d listens", got)
	}
}

// TestStartFileEventListenerBusMode covers the pure-bus transport: the app never connects NATS,
// subscribes to the base interest topic over the bus, filters matched.listeners locally and
// receives the decoded event. Delivery acknowledgement is daemon-side, so there is no NATS ACK or
// result report in this path.
func TestStartFileEventListenerBusMode(t *testing.T) {
	const (
		appID      = "com.example.album"
		listenerID = "lst-bus"
	)
	srv := bustest.New(t, bustest.Options{
		AppID: appID,
		OnCall: func(req protocol.Request) (string, json.RawMessage) {
			switch string(req.Template) {
			case protocol.TemplateInterestListen:
				data, _ := json.Marshal(interestListenResponse{
					ListenerID: listenerID, Transport: TransportBus,
					BusTopic: protocol.TopicInterestEvent + "." + appID, AppCode: appID,
				})
				return protocol.CodeSuccess, data
			case protocol.TemplateInterestStop, protocol.TemplateInterestRenew:
				return protocol.CodeSuccess, json.RawMessage(`{}`)
			}
			return "", nil
		},
	})
	client := bus.New(bus.Options{WSURL: srv.URL()})
	defer client.Close()

	got := make(chan FileEvent, 4)
	listener, err := StartFileEventListener(context.Background(), client, FileEventListenerOptions{
		Dirs: []string{"/Photos"}, Transport: TransportBus,
	}, func(ev FileEvent) error {
		got <- ev
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()

	if listener.Transport() != TransportBus {
		t.Fatalf("transport = %q, want %q", listener.Transport(), TransportBus)
	}
	// The listen request must carry the requested transport (the daemon defaults to nats otherwise).
	call, ok := srv.LastCall(protocol.TemplateInterestListen)
	if !ok {
		t.Fatal("no interest listen call reached the fake daemon")
	}
	if call["transport"] != TransportBus {
		t.Fatalf("listen transport = %v, want %q", call["transport"], TransportBus)
	}

	// The SDK subscribes to the base template over the bus; the daemon expands it per app.
	srv.WaitForSubscription(t, protocol.TopicInterestEvent)

	// An event for another listener of the same app is filtered out.
	other := map[string]any{
		"v": 2, "eventId": "evt-other", "ufi": "/ufi/pool-raw/syspool/Photos/other.jpg",
		"action": 1, "inode": 2,
		"ugn":     map[string]any{"username": "alice", "groupName": "", "namespace": "main"},
		"matched": map[string]any{"apps": []string{appID}, "listeners": []string{"someone-else"}},
	}
	if n := srv.Publish(t, protocol.TopicInterestEvent, other); n != 1 {
		t.Fatalf("delivered connections = %d, want 1", n)
	}

	payload := map[string]any{
		"v": 2, "eventId": "evt-bus-1", "ufi": "/ufi/pool-raw/syspool/Photos/a.jpg",
		"action": 1, "inode": 7,
		"ugn":     map[string]any{"username": "alice", "groupName": "", "namespace": "main"},
		"matched": map[string]any{"apps": []string{appID}, "listeners": []string{listenerID}},
	}
	if n := srv.Publish(t, protocol.TopicInterestEvent, payload); n != 1 {
		t.Fatalf("delivered connections = %d, want 1", n)
	}

	select {
	case ev := <-got:
		if ev.UFI != "/ufi/pool-raw/syspool/Photos/a.jpg" || ev.Action != 1 || ev.ActionName != "add" || ev.Inode != 7 {
			t.Fatalf("unexpected event: %+v", ev)
		}
		if ev.Space() != "alice-main" {
			t.Fatalf("Space() = %q, want alice-main", ev.Space())
		}
		if ev.ReceivedAt == "" {
			t.Fatal("ReceivedAt must be filled in by the SDK")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("bus-mode listener did not receive the event")
	}
	// The non-matching event must not be dispatched either before or after the matching one.
	select {
	case ev := <-got:
		t.Fatalf("event of another listener must be filtered out, got %+v", ev)
	case <-time.After(200 * time.Millisecond):
	}

	// Close stops the listener through the daemon (and drops the bus subscription).
	listener.Close()
	if _, ok := srv.LastCall(protocol.TemplateInterestStop); !ok {
		t.Fatal("Close must ask the daemon to stop the listener")
	}
}

// TestStartFileEventListenerRejectsUnknownTransport: no silent fallback to a different transport.
func TestStartFileEventListenerRejectsUnknownTransport(t *testing.T) {
	client := bus.New(bus.Options{WSURL: "ws://127.0.0.1:1/ws"})
	defer client.Close()
	_, err := StartFileEventListener(context.Background(), client, FileEventListenerOptions{
		Dirs: []string{"/Photos"}, Transport: "carrier-pigeon",
	}, func(FileEvent) error { return nil })
	if err == nil {
		t.Fatal("an unsupported transport must be rejected")
	}
}

// TestBusModeMultipleListenersShareEventPump: all capability-level subscriptions of one bus client
// share a single event pump, so two bus-mode listeners on the same app each receive their own
// events (per-subscription readers would steal each other's events from the one event channel).
func TestBusModeMultipleListenersShareEventPump(t *testing.T) {
	const appID = "com.example.album"
	var nextID atomic.Int32
	srv := bustest.New(t, bustest.Options{
		AppID: appID,
		OnCall: func(req protocol.Request) (string, json.RawMessage) {
			switch string(req.Template) {
			case protocol.TemplateInterestListen:
				data, _ := json.Marshal(interestListenResponse{
					ListenerID: fmt.Sprintf("lst-%d", nextID.Add(1)), Transport: TransportBus,
					BusTopic: protocol.TopicInterestEvent + "." + appID, AppCode: appID,
				})
				return protocol.CodeSuccess, data
			case protocol.TemplateInterestStop, protocol.TemplateInterestRenew:
				return protocol.CodeSuccess, json.RawMessage(`{}`)
			}
			return "", nil
		},
	})
	client := bus.New(bus.Options{WSURL: srv.URL()})
	defer client.Close()

	first := make(chan FileEvent, 4)
	second := make(chan FileEvent, 4)
	l1, err := StartFileEventListener(context.Background(), client, FileEventListenerOptions{
		Dirs: []string{"/Photos"}, Transport: TransportBus,
	}, func(ev FileEvent) error { first <- ev; return nil })
	if err != nil {
		t.Fatal(err)
	}
	defer l1.Close()
	l2, err := StartFileEventListener(context.Background(), client, FileEventListenerOptions{
		Dirs: []string{"/Music"}, Transport: TransportBus,
	}, func(ev FileEvent) error { second <- ev; return nil })
	if err != nil {
		t.Fatal(err)
	}
	defer l2.Close()
	srv.WaitForSubscription(t, protocol.TopicInterestEvent)

	event := func(listenerID, ufi string) map[string]any {
		return map[string]any{
			"v": 2, "eventId": "evt-" + listenerID, "ufi": ufi, "action": 1, "inode": 1,
			"ugn":     map[string]any{"username": "alice", "groupName": "", "namespace": "main"},
			"matched": map[string]any{"apps": []string{appID}, "listeners": []string{listenerID}},
		}
	}
	if n := srv.Publish(t, protocol.TopicInterestEvent, event(l1.ListenerID(), "/ufi/pool-raw/syspool/Photos/a.jpg")); n != 1 {
		t.Fatalf("delivered connections = %d, want 1", n)
	}
	if n := srv.Publish(t, protocol.TopicInterestEvent, event(l2.ListenerID(), "/ufi/pool-raw/syspool/Music/b.flac")); n != 1 {
		t.Fatalf("delivered connections = %d, want 1", n)
	}

	select {
	case ev := <-first:
		if ev.UGN.Username != "alice" || ev.UFI != "/ufi/pool-raw/syspool/Photos/a.jpg" {
			t.Fatalf("listener 1 got %+v", ev)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("listener 1 received no event")
	}
	select {
	case ev := <-second:
		if ev.UFI != "/ufi/pool-raw/syspool/Music/b.flac" {
			t.Fatalf("listener 2 got %+v", ev)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("listener 2 received no event")
	}
	// Neither listener may see the other's event.
	select {
	case ev := <-first:
		t.Fatalf("listener 1 got another listener's event: %+v", ev)
	case ev := <-second:
		t.Fatalf("listener 2 got another listener's event: %+v", ev)
	case <-time.After(200 * time.Millisecond):
	}
}

// TestBusModeRecoversAfterRenewFailure: bus mode re-registers the listener on a renew failure and
// keeps consuming over the existing (app-scoped) bus subscription with the new listener id.
func TestBusModeRecoversAfterRenewFailure(t *testing.T) {
	const appID = "com.example.album"
	var listens atomic.Int32
	srv := bustest.New(t, bustest.Options{
		AppID: appID,
		OnCall: func(req protocol.Request) (string, json.RawMessage) {
			switch string(req.Template) {
			case protocol.TemplateInterestListen:
				n := listens.Add(1)
				data, _ := json.Marshal(interestListenResponse{
					ListenerID: fmt.Sprintf("lst-%d", n), Transport: TransportBus,
					BusTopic: protocol.TopicInterestEvent + "." + appID, AppCode: appID,
				})
				return protocol.CodeSuccess, data
			case protocol.TemplateInterestRenew:
				return protocol.CodeError, nil
			case protocol.TemplateInterestStop:
				return protocol.CodeSuccess, json.RawMessage(`{}`)
			}
			return "", nil
		},
	})
	client := bus.New(bus.Options{WSURL: srv.URL()})
	defer client.Close()

	got := make(chan FileEvent, 4)
	listener, err := StartFileEventListener(context.Background(), client, FileEventListenerOptions{
		Dirs: []string{"/Photos"}, Transport: TransportBus, TTL: 300 * time.Millisecond,
	}, func(ev FileEvent) error { got <- ev; return nil })
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	srv.WaitForSubscription(t, protocol.TopicInterestEvent)

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) && listens.Load() < 2 {
		time.Sleep(20 * time.Millisecond)
	}
	if got := listens.Load(); got < 2 {
		t.Fatalf("renew failure should trigger a new listen, got %d listens", got)
	}
	newID := listener.ListenerID()
	payload := map[string]any{
		"v": 2, "eventId": "evt-recovered", "ufi": "/ufi/pool-raw/syspool/Photos/x.jpg",
		"action": 1, "inode": 1,
		"ugn":     map[string]any{"username": "alice", "groupName": "", "namespace": "main"},
		"matched": map[string]any{"apps": []string{appID}, "listeners": []string{newID}},
	}
	if n := srv.Publish(t, protocol.TopicInterestEvent, payload); n != 1 {
		t.Fatalf("delivered connections = %d, want 1", n)
	}
	select {
	case ev := <-got:
		if ev.UFI != "/ufi/pool-raw/syspool/Photos/x.jpg" {
			t.Fatalf("unexpected event after recovery: %+v", ev)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("listener did not consume after recovering")
	}
}

func TestInterestListGetUpdate(t *testing.T) {
	const appID = "com.example.album"
	const listenerID = "lst-crud"
	srv := bustest.New(t, bustest.Options{
		AppID: appID,
		OnCall: func(req protocol.Request) (string, json.RawMessage) {
			switch string(req.Template) {
			case protocol.TemplateInterestList:
				data, _ := json.Marshal(map[string]any{
					"items": []InterestListenerInfo{{ListenerID: listenerID, AppCode: appID, Transport: TransportBus, Dirs: []string{"/Photos"}}},
					"total": 1,
				})
				return protocol.CodeSuccess, data
			case protocol.TemplateInterestGet:
				data, _ := json.Marshal(InterestListenerInfo{ListenerID: listenerID, AppCode: appID, Transport: TransportBus, Dirs: []string{"/Photos"}})
				return protocol.CodeSuccess, data
			case protocol.TemplateInterestUpdate:
				var payload struct {
					ListenerID string   `json:"listenerId"`
					Dirs       []string `json:"dirs"`
				}
				_ = json.Unmarshal(req.Payload, &payload)
				data, _ := json.Marshal(InterestListenerInfo{ListenerID: payload.ListenerID, AppCode: appID, Transport: TransportBus, Dirs: payload.Dirs})
				return protocol.CodeSuccess, data
			}
			return "", nil
		},
	})
	client := bus.New(bus.Options{WSURL: srv.URL()})
	defer client.Close()
	ctx := context.Background()

	items, err := ListFileEventListeners(ctx, client)
	if err != nil || len(items) != 1 || items[0].ListenerID != listenerID {
		t.Fatalf("list: %+v, err=%v", items, err)
	}
	info, err := GetFileEventListener(ctx, client, listenerID)
	if err != nil || info.ListenerID != listenerID {
		t.Fatalf("get: %+v, err=%v", info, err)
	}

	// Update requires a listener handle; construct one against the same client.
	l := &FileEventListener{client: client, listenerID: listenerID, transport: TransportBus}
	if err := l.Update(ctx, FileEventListenerOptions{Dirs: []string{"/Movies"}}); err != nil {
		t.Fatalf("update: %v", err)
	}
	if got := l.opts.Dirs; len(got) != 1 || got[0] != "/Movies" {
		t.Fatalf("local opts not updated: %+v", l.opts)
	}
}
