package bus

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"strconv"
	"testing"
	"time"

	"github.com/siyouyun-open/siyouyun_sdk/v2/protocol"
)

// PushReliableEvent pushes one event carrying a delivery id (what a daemon does for a
// reliable subscription).
func (f *fakeBus) PushReliableEvent(topic string, payload any, eventID string) {
	raw, _ := json.Marshal(payload)
	resp := protocol.Response{Type: protocol.FrameEvent, ID: "evt", Topic: topic, Event: raw, EventID: eventID}
	f.mu.Lock()
	conns := append([]net.Conn(nil), f.conns...)
	f.mu.Unlock()
	for _, conn := range conns {
		f.write(conn, resp)
	}
}

func (f *fakeBus) AckEventIDs() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	var ids []string
	for _, req := range f.requests {
		if req.Type == protocol.TypeAck {
			ids = append(ids, req.EventID)
		}
	}
	return ids
}

func (f *fakeBus) countSubscribes(reliable bool) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	n := 0
	for _, req := range f.requests {
		if req.Type == protocol.TypeSubscribe && req.Reliable == reliable {
			n++
		}
	}
	return n
}

func waitForCondition(t *testing.T, timeout time.Duration, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", what)
}

// TestAckFrameWireFormat pins the JSON shape of the two new frames. It is the contract the
// daemon implements, so the field names are asserted literally (an SDK/daemon mismatch here
// would only show up as "events are redelivered forever" on a real device).
func TestAckFrameWireFormat(t *testing.T) {
	raw, err := json.Marshal(protocol.Request{Type: protocol.TypeAck, ID: "ack-1", EventID: "17"})
	if err != nil {
		t.Fatalf("marshal ack: %v", err)
	}
	var ackFields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &ackFields); err != nil {
		t.Fatalf("decode ack fields: %v", err)
	}
	var ackType string
	if err := json.Unmarshal(ackFields["type"], &ackType); err != nil || ackType != "ack" {
		t.Fatalf(`ack frame type = %s (err=%v), want "ack"`, ackFields["type"], err)
	}
	var ackEventID string
	if err := json.Unmarshal(ackFields["event_id"], &ackEventID); err != nil || ackEventID != "17" {
		t.Fatalf(`ack frame event_id = %s (err=%v), want "17"`, ackFields["event_id"], err)
	}

	// Decoding direction: the daemon-side parser reads the same frames.
	var decoded protocol.Request
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("decode ack: %v", err)
	}
	if decoded.Type != protocol.TypeAck || decoded.EventID != "17" || decoded.ID != "ack-1" {
		t.Fatalf("decoded ack = %+v", decoded)
	}

	// Event pushes for reliable subscriptions carry the delivery id in event_id.
	push, err := json.Marshal(protocol.Response{
		Type:    protocol.FrameEvent,
		Topic:   protocol.TopicFileEvent + ".com.example.app",
		Event:   json.RawMessage(`{"action":1}`),
		EventID: "17",
	})
	if err != nil {
		t.Fatalf("marshal event: %v", err)
	}
	var pushFields map[string]json.RawMessage
	if err := json.Unmarshal(push, &pushFields); err != nil {
		t.Fatalf("decode event fields: %v", err)
	}
	var pushEventID string
	if err := json.Unmarshal(pushFields["event_id"], &pushEventID); err != nil || pushEventID != "17" {
		t.Fatalf(`event frame event_id = %s (err=%v), want "17"`, pushFields["event_id"], err)
	}
	var decodedPush protocol.Response
	if err := json.Unmarshal(push, &decodedPush); err != nil {
		t.Fatalf("decode event: %v", err)
	}
	if decodedPush.EventID != "17" || decodedPush.Type != protocol.FrameEvent {
		t.Fatalf("decoded event push = %+v", decodedPush)
	}
}

// TestSubscribeReliableAndAck covers the happy path: the subscribe frame asks for reliable
// delivery, a delivery on the reliable channel is not leaked into the hint channel, and Ack
// writes a frame carrying the delivery id.
func TestSubscribeReliableAndAck(t *testing.T) {
	fake := newFakeBus(t)
	client := New(testOptions(fake))
	defer client.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := client.SubscribeReliable(ctx, protocol.TopicFileEvent); err != nil {
		t.Fatalf("SubscribeReliable: %v", err)
	}
	if n := fake.countSubscribes(true); n != 1 {
		t.Fatalf("reliable subscribe frames = %d, want 1", n)
	}

	fake.PushReliableEvent(protocol.TopicFileEvent+".com.example.app", map[string]any{"action": 1, "ufi": "/ufi/x"}, "17")

	select {
	case event := <-client.ReliableEvents():
		if event.ID != "17" {
			t.Fatalf("event id = %q, want 17", event.ID)
		}
		if event.Topic != protocol.TopicFileEvent+".com.example.app" {
			t.Fatalf("event topic = %q", event.Topic)
		}
		if err := client.Ack(event.ID); err != nil {
			t.Fatalf("Ack: %v", err)
		}
	case <-ctx.Done():
		t.Fatal("timed out waiting for reliable event")
	}

	waitForCondition(t, 2*time.Second, "ack frame", func() bool {
		ids := fake.AckEventIDs()
		return len(ids) == 1 && ids[0] == "17"
	})
	if stats := client.Stats(); stats.AcksSent != 1 || stats.AcksFailed != 0 {
		t.Fatalf("stats = %+v", stats)
	}

	// Reliable deliveries must not consume the bounded fire-and-forget channel.
	select {
	case event := <-client.Events():
		t.Fatalf("reliable delivery leaked into Events(): %+v", event)
	default:
	}
}

// TestSubscribeReliableRejectsOldDaemon: an older daemon ignores `reliable` and confirms a
// plain subscription. Reporting success there would silently downgrade the caller to a lossy
// subscription, so the client must fail loudly and the caller can fall back deliberately.
func TestSubscribeReliableRejectsOldDaemon(t *testing.T) {
	fake := newFakeBus(t)
	fake.reliableUnsupported = true
	client := New(testOptions(fake))
	defer client.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err := client.SubscribeReliable(ctx, protocol.TopicFileEvent)
	if !errors.Is(err, ErrReliableUnsupported) {
		t.Fatalf("SubscribeReliable error = %v, want ErrReliableUnsupported", err)
	}

	// The default fire-and-forget subscription is unaffected.
	if err := client.Subscribe(ctx, protocol.TopicNotificationEvent); err != nil {
		t.Fatalf("plain Subscribe on an old daemon: %v", err)
	}
}

// TestReliableDeliveryBypassesFullEventBuffer: the bounded hint channel is allowed to drop, the
// reliable path is not. A reliable delivery must arrive even when Events() is completely
// backed up.
func TestReliableDeliveryBypassesFullEventBuffer(t *testing.T) {
	fake := newFakeBus(t)
	opts := testOptions(fake)
	opts.EventBuffer = 1
	client := New(opts)
	defer client.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := client.SubscribeReliable(ctx, protocol.TopicFileEvent); err != nil {
		t.Fatalf("SubscribeReliable: %v", err)
	}

	// Fill (and overflow) the hint channel; nobody reads it.
	for i := 0; i < 5; i++ {
		fake.PushEvent(protocol.TopicFileEvent, map[string]any{"hint": i})
	}
	fake.PushReliableEvent(protocol.TopicFileEvent, map[string]any{"action": 1}, "7")

	select {
	case event := <-client.ReliableEvents():
		if event.ID != "7" {
			t.Fatalf("event id = %q, want 7", event.ID)
		}
	case <-ctx.Done():
		t.Fatal("reliable delivery was lost while the fire-and-forget buffer was full")
	}
	if stats := client.Stats(); stats.ReliableDropped != 0 {
		t.Fatalf("ReliableDropped = %d, want 0", stats.ReliableDropped)
	}
}

// TestReliableQueueOverflowDropsWithoutAck documents the overflow semantics: the local queue is
// bounded, an overflow drops the delivery **without acknowledging it** (so the daemon
// redelivers it after AckWait) and is counted in Stats.
func TestReliableQueueOverflowDropsWithoutAck(t *testing.T) {
	fake := newFakeBus(t)
	opts := testOptions(fake)
	opts.ReliableQueueLimit = 2
	client := New(opts)
	defer client.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Connect without starting the reliable pump (a plain subscribe is enough to have a
	// session), so the queue fills deterministically.
	if err := client.Subscribe(ctx, protocol.TopicFileEvent); err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	for i := 1; i <= 4; i++ {
		fake.PushReliableEvent(protocol.TopicFileEvent, map[string]any{"n": i}, strconv.Itoa(i))
	}
	waitForCondition(t, 2*time.Second, "two reliable deliveries to be dropped", func() bool {
		return client.Stats().ReliableDropped == 2
	})

	events := client.ReliableEvents()
	for _, want := range []string{"1", "2"} {
		select {
		case event := <-events:
			if event.ID != want {
				t.Fatalf("event id = %q, want %q", event.ID, want)
			}
		case <-ctx.Done():
			t.Fatalf("timed out waiting for queued event %s", want)
		}
	}
	// Dropped deliveries must not have been acknowledged: that is what makes the daemon
	// redeliver them instead of losing them.
	if ids := fake.AckEventIDs(); len(ids) != 0 {
		t.Fatalf("dropped deliveries were acked: %v", ids)
	}
}

// TestReconnectReplaysReliableSubscription: after a disconnect the client reconnects, replays
// the subscription as reliable, and reliable deliveries work on the new connection.
func TestReconnectReplaysReliableSubscription(t *testing.T) {
	fake := newFakeBus(t)
	client := New(testOptions(fake))
	defer client.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := client.SubscribeReliable(ctx, protocol.TopicFileEvent); err != nil {
		t.Fatalf("SubscribeReliable: %v", err)
	}

	fake.DropConns()
	waitForCondition(t, 3*time.Second, "reliable subscription replay after reconnect", func() bool {
		return fake.countSubscribes(true) >= 2
	})

	fake.PushReliableEvent(protocol.TopicFileEvent, map[string]any{"action": 1}, "9")
	select {
	case event := <-client.ReliableEvents():
		if event.ID != "9" {
			t.Fatalf("event id = %q, want 9", event.ID)
		}
	case <-ctx.Done():
		t.Fatal("no reliable delivery after reconnect")
	}
}

// TestAckWithoutConnection: acknowledging does not dial. The daemon redelivers the delivery, so
// a failed ACK is reported and counted rather than retried against a dead connection.
func TestAckWithoutConnection(t *testing.T) {
	client := New(Options{})
	defer client.Close()

	if err := client.Ack(""); err == nil {
		t.Fatal("Ack with an empty event id must fail")
	}
	err := client.Ack("17")
	if !errors.Is(err, ErrNotConnected) {
		t.Fatalf("Ack error = %v, want ErrNotConnected", err)
	}
	if stats := client.Stats(); stats.AcksFailed != 1 || stats.AcksSent != 0 {
		t.Fatalf("stats = %+v", stats)
	}
}
