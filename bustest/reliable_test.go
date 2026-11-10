package bustest

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/siyouyun-open/siyouyun_sdk/v2/bus"
	"github.com/siyouyun-open/siyouyun_sdk/v2/capability"
	"github.com/siyouyun-open/siyouyun_sdk/v2/protocol"
)

func fileEventPayload(ufi string) map[string]any {
	return map[string]any{
		"ugn":    map[string]any{"username": "alice", "groupName": "", "namespace": "main"},
		"ufi":    ufi,
		"inode":  42,
		"action": 1,
	}
}

func newReliableTestClient(srv *Server) *bus.Client {
	return bus.New(bus.Options{
		WSURL:       srv.URL(),
		MinBackoff:  10 * time.Millisecond,
		MaxBackoff:  50 * time.Millisecond,
		DialTimeout: 2 * time.Second,
	})
}

// TestReliableSubscriptionAckPreventsRedelivery: handler success -> SDK ACKs -> the fake daemon
// stops redelivering. This is the core at-least-once path: if the ACK never arrived, the next
// AckWait tick would push the event again.
func TestReliableSubscriptionAckPreventsRedelivery(t *testing.T) {
	srv := New(t, Options{AppID: "album", AckWait: 80 * time.Millisecond})
	client := newReliableTestClient(srv)
	defer func() { _ = client.Close() }()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	received := make(chan capability.FileEvent, 4)
	sub, err := capability.SubscribeFileEventsReliable(ctx, client, func(ev capability.FileEvent) error {
		select {
		case received <- ev:
		default:
		}
		return nil
	})
	if err != nil {
		t.Fatalf("SubscribeFileEventsReliable: %v", err)
	}
	defer sub.Close()
	srv.WaitForSubscription(t, protocol.TopicFileEvent)

	id := srv.PublishReliable(t, protocol.TopicFileEvent, fileEventPayload("/ufi/pool-raw/syspool/Photos/a.jpg"))
	select {
	case ev := <-received:
		if ev.Action != 1 || ev.ActionName != "add" {
			t.Fatalf("action = %d/%q", ev.Action, ev.ActionName)
		}
		if ev.Space() != "alice-main" {
			t.Fatalf("Space() = %q (the reliable path must decode the producer shape exactly like the plain one)", ev.Space())
		}
	case <-time.After(3 * time.Second):
		t.Fatal("no reliable event received")
	}

	srv.WaitForAck(t, id)
	if n := srv.Deliveries(id); n != 1 {
		t.Fatalf("deliveries before ack = %d, want 1", n)
	}
	time.Sleep(4 * 80 * time.Millisecond) // several AckWait periods
	if n := srv.Deliveries(id); n != 1 {
		t.Fatalf("deliveries after ack = %d, want 1 (an acked event must not be redelivered)", n)
	}
	if n := srv.PendingCount(); n != 0 {
		t.Fatalf("pending deliveries = %d, want 0", n)
	}
	if stats := client.Stats(); stats.AcksSent != 1 || stats.AcksFailed != 0 {
		t.Fatalf("stats = %+v", stats)
	}
}

// TestReliableHandlerErrorTriggersRedelivery: handler failure -> no ACK -> the daemon redelivers
// after AckWait -> the second attempt succeeds and is acked.
func TestReliableHandlerErrorTriggersRedelivery(t *testing.T) {
	srv := New(t, Options{AppID: "album", AckWait: 80 * time.Millisecond})
	client := newReliableTestClient(srv)
	defer func() { _ = client.Close() }()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var attempts atomic.Int32
	seen := make(chan capability.FileEvent, 8)
	sub, err := capability.SubscribeFileEventsReliable(ctx, client, func(ev capability.FileEvent) error {
		n := attempts.Add(1)
		select {
		case seen <- ev:
		default:
		}
		if n == 1 {
			return errors.New("database is temporarily locked")
		}
		return nil
	})
	if err != nil {
		t.Fatalf("SubscribeFileEventsReliable: %v", err)
	}
	defer sub.Close()
	srv.WaitForSubscription(t, protocol.TopicFileEvent)

	id := srv.PublishReliable(t, protocol.TopicFileEvent, fileEventPayload("/ufi/pool-raw/syspool/Photos/b.jpg"))
	srv.WaitForDeliveries(t, id, 2)
	srv.WaitForAck(t, id)

	if n := attempts.Load(); n < 2 {
		t.Fatalf("handler attempts = %d, want at least 2 (the failed attempt must be redelivered)", n)
	}
	// The same event is delivered again on redelivery: the handler must be idempotent.
	if len(seen) < 2 {
		t.Fatalf("handler saw %d deliveries, want at least 2", len(seen))
	}
	time.Sleep(4 * 80 * time.Millisecond)
	if n := srv.Deliveries(id); n != 2 {
		t.Fatalf("deliveries after ack = %d, want exactly 2", n)
	}
	if n := srv.PendingCount(); n != 0 {
		t.Fatalf("pending deliveries = %d, want 0", n)
	}
}

// TestReliableRedeliversAfterReconnect: the unacknowledged event survives the connection loss.
// After reconnecting, the SDK replays the reliable subscription and the fake daemon re-pushes
// everything that is still pending (what a JetStream durable consumer does).
func TestReliableRedeliversAfterReconnect(t *testing.T) {
	srv := New(t, Options{AppID: "album", AckWait: 10 * time.Second}) // no timer redelivery during the test
	client := newReliableTestClient(srv)
	defer func() { _ = client.Close() }()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var attempts atomic.Int32
	sub, err := capability.SubscribeFileEventsReliable(ctx, client, func(ev capability.FileEvent) error {
		if attempts.Add(1) == 1 {
			return errors.New("not yet")
		}
		return nil
	})
	if err != nil {
		t.Fatalf("SubscribeFileEventsReliable: %v", err)
	}
	defer sub.Close()
	srv.WaitForSubscription(t, protocol.TopicFileEvent)

	id := srv.PublishReliable(t, protocol.TopicFileEvent, fileEventPayload("/ufi/pool-raw/syspool/Photos/c.jpg"))
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) && attempts.Load() < 1 {
		time.Sleep(5 * time.Millisecond)
	}
	if attempts.Load() < 1 {
		t.Fatal("handler was never called before the disconnect")
	}

	srv.DropConnections()
	srv.WaitForDeliveries(t, id, 2) // replayed subscription -> pending event re-pushed
	srv.WaitForAck(t, id)

	if n := attempts.Load(); n < 2 {
		t.Fatalf("handler attempts after reconnect = %d, want at least 2", n)
	}
	if n := srv.PendingCount(); n != 0 {
		t.Fatalf("pending deliveries = %d, want 0", n)
	}
}

// TestReliablePoisonEventIsAckedAndSkipped: a body that cannot be decoded is acknowledged and
// skipped. Not acking it would make the daemon redeliver the same undecodable event for the
// whole retention window (one hour on the v2 daemon).
func TestReliablePoisonEventIsAckedAndSkipped(t *testing.T) {
	srv := New(t, Options{AppID: "album", AckWait: 60 * time.Millisecond})
	client := newReliableTestClient(srv)
	defer func() { _ = client.Close() }()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var calls atomic.Int32
	sub, err := capability.SubscribeFileEventsReliable(ctx, client, func(ev capability.FileEvent) error {
		calls.Add(1)
		return nil
	})
	if err != nil {
		t.Fatalf("SubscribeFileEventsReliable: %v", err)
	}
	defer sub.Close()
	srv.WaitForSubscription(t, protocol.TopicFileEvent)

	// A body that is valid JSON but not a file event: the SDK cannot ever process it.
	id := srv.PublishReliable(t, protocol.TopicFileEvent, "not-a-file-event")
	srv.WaitForAck(t, id)
	if calls.Load() != 0 {
		t.Fatalf("handler calls = %d, want 0 for an undecodable event", calls.Load())
	}
	time.Sleep(4 * 60 * time.Millisecond)
	if n := srv.Deliveries(id); n != 1 {
		t.Fatalf("deliveries = %d, want 1 (an undecodable event must not be redelivered)", n)
	}
}

// TestPlainPublishDoesNotTargetReliableSubscription pins the fake daemon's split: a plain
// Publish is not a tracked delivery, so a reliable subscription (which only reads tracked
// deliveries) must use PublishReliable.
func TestPlainPublishDoesNotTargetReliableSubscription(t *testing.T) {
	srv := New(t, Options{AppID: "album"})
	client := newReliableTestClient(srv)
	defer func() { _ = client.Close() }()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sub, err := capability.SubscribeFileEventsReliable(ctx, client, func(capability.FileEvent) error { return nil })
	if err != nil {
		t.Fatalf("SubscribeFileEventsReliable: %v", err)
	}
	defer sub.Close()
	srv.WaitForSubscription(t, protocol.TopicFileEvent)

	if n := srv.Publish(t, protocol.TopicFileEvent, fileEventPayload("/ufi/x")); n != 0 {
		t.Fatalf("plain Publish delivered to %d reliable subscriptions, want 0", n)
	}
}
