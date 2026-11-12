package bustest

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/siyouyun-open/siyouyun_sdk/v2/bus"
	"github.com/siyouyun-open/siyouyun_sdk/v2/protocol"
)

// TestSubscribeReceivesPublishedEvent is the "fake daemon"'s own acceptance test:
// a real bus.Client subscribes to the pure-bus interest topic → the daemon expansion is applied
// → the event frame is pushed → the raw OS event body survives the round trip verbatim.
func TestSubscribeReceivesPublishedEvent(t *testing.T) {
	srv := New(t, Options{AppID: "album"})
	client := bus.New(bus.Options{WSURL: srv.URL()})
	defer func() { _ = client.Close() }()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := client.Subscribe(ctx, protocol.TopicInterestEvent); err != nil {
		t.Fatalf("Subscribe: %v", err)
	}

	srv.WaitForSubscription(t, protocol.TopicInterestEvent)

	// Publish an event in the **producer's real shape**: the OS interest event is
	// {v, eventId, ufi, srcUfi, action, inode, ugn, matched}.
	payload := map[string]any{
		"v":       2,
		"eventId": "evt-1",
		"ugn":     map[string]any{"username": "alice", "groupName": "", "namespace": "main"},
		"srcUfi":  "/ufi/pool-raw/syspool/Photos/old.jpg",
		"ufi":     "/ufi/pool-raw/syspool/Photos/new.jpg",
		"inode":   42,
		"action":  3,
		"matched": map[string]any{"apps": []string{"album"}, "listeners": []string{"lst-1"}},
	}
	if n := srv.Publish(t, protocol.TopicInterestEvent, payload); n != 1 {
		t.Fatalf("delivered connections = %d, want 1", n)
	}

	select {
	case event := <-client.Events():
		if event.Topic != protocol.TopicInterestEvent+".album" {
			t.Fatalf("topic = %q, want the expanded %q", event.Topic, protocol.TopicInterestEvent+".album")
		}
		var got struct {
			EventID string `json:"eventId"`
			Action  int    `json:"action"`
			UFI     string `json:"ufi"`
			Inode   uint64 `json:"inode"`
			UGN     struct {
				Username string `json:"username"`
			} `json:"ugn"`
			Matched struct {
				Listeners []string `json:"listeners"`
			} `json:"matched"`
		}
		if err := json.Unmarshal(event.Data, &got); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if got.Action != 3 || got.EventID != "evt-1" || got.Inode != 42 || got.UGN.Username != "alice" {
			t.Fatalf("unexpected event: %+v", got)
		}
		if len(got.Matched.Listeners) != 1 || got.Matched.Listeners[0] != "lst-1" {
			t.Fatalf("matched listeners = %v", got.Matched.Listeners)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("no event received")
	}
}

// TestPublishSkipsUnsubscribed: a connection that did not subscribe must not receive events
// (otherwise problems like "filtering did not take effect" would be invisible in tests too).
func TestPublishSkipsUnsubscribed(t *testing.T) {
	srv := New(t, Options{AppID: "album"})
	if n := srv.Publish(t, protocol.TopicInterestEvent, map[string]any{"action": 1}); n != 0 {
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
