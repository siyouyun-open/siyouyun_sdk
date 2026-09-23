package bustest

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/siyouyun-open/siyouyun_sdk/v2/bus"
	"github.com/siyouyun-open/siyouyun_sdk/v2/capability"
	"github.com/siyouyun-open/siyouyun_sdk/v2/protocol"
)

// TestSubscribeReceivesPublishedEvent is the "fake daemon"'s own acceptance test:
// a real bus.Client subscribes → receives an event frame → decodes it into a
// capability.FileEvent.
func TestSubscribeReceivesPublishedEvent(t *testing.T) {
	srv := New(t, Options{AppID: "album"})
	client := bus.New(bus.Options{WSURL: srv.URL()})
	defer func() { _ = client.Close() }()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	got := make(chan capability.FileEvent, 1)
	sub, err := capability.SubscribeFileEvents(ctx, client, func(ev capability.FileEvent) {
		select {
		case got <- ev:
		default:
		}
	})
	if err != nil {
		t.Fatalf("SubscribeFileEvents: %v", err)
	}
	defer sub.Close()

	srv.WaitForSubscription(t, protocol.TopicFileEvent)

	// Publish an event in the **producer's real shape**: a platform file event is
	// {ugn, srcUfi, ufi, inode, action, …}.
	payload := map[string]any{
		"ugn":    map[string]any{"username": "alice", "groupName": "", "namespace": "main"},
		"srcUfi": "/ufi/pool-raw/syspool/Photos/old.jpg",
		"ufi":    "/ufi/pool-raw/syspool/Photos/new.jpg",
		"inode":  42,
		"action": 3,
	}
	if n := srv.Publish(t, protocol.TopicFileEvent, payload); n != 1 {
		t.Fatalf("delivered connections = %d, want 1", n)
	}

	select {
	case ev := <-got:
		if ev.Action != 3 || ev.ActionName != "rename" {
			t.Fatalf("action = %d/%q", ev.Action, ev.ActionName)
		}
		if ev.Space() != "alice-main" {
			t.Fatalf("Space() = %q, want alice-main (without decoding the UGN the event is routed to the wrong space)", ev.Space())
		}
		if ev.SrcUFI != "/ufi/pool-raw/syspool/Photos/old.jpg" {
			t.Fatalf("SrcUFI = %q", ev.SrcUFI)
		}
		if ev.Inode != 42 || ev.ReceivedAt == "" {
			t.Fatalf("inode=%d receivedAt=%q", ev.Inode, ev.ReceivedAt)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("no event received")
	}
}

// TestPublishSkipsUnsubscribed: a connection that did not subscribe must not receive events
// (otherwise problems like "filtering did not take effect" would be invisible in tests too).
func TestPublishSkipsUnsubscribed(t *testing.T) {
	srv := New(t, Options{AppID: "album"})
	if n := srv.Publish(t, protocol.TopicFileEvent, map[string]any{"action": 1}); n != 0 {
		t.Fatalf("deliveries with no subscriber = %d, want 0", n)
	}
}

// TestCallIsAcked: the application's other capability calls (KV / media info) go over the same
// connection, and the fake daemon must reply, otherwise calls during startup time out.
func TestCallIsAcked(t *testing.T) {
	srv := New(t, Options{AppID: "album", CallData: json.RawMessage(`{"ok":true}`)})
	client := bus.New(bus.Options{WSURL: srv.URL()})
	defer func() { _ = client.Close() }()

	var out struct {
		OK bool `json:"ok"`
	}
	if err := client.CallInto(context.Background(), &bus.Request{Template: protocol.TemplateKVGet}, &out); err != nil {
		t.Fatalf("CallInto: %v", err)
	}
	if !out.OK {
		t.Fatalf("response was not decoded: %+v", out)
	}
	if srv.CallCount() != 1 {
		t.Fatalf("call count = %d, want 1", srv.CallCount())
	}
}
