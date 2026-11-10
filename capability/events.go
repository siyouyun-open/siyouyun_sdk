package capability

import (
	"context"
	"encoding/json"
	"sync"
	"time"

	"github.com/siyouyun-open/siyouyun_sdk/v2/appconst"
	"github.com/siyouyun-open/siyouyun_sdk/v2/bus"
	appfs "github.com/siyouyun-open/siyouyun_sdk/v2/fs"
	"github.com/siyouyun-open/siyouyun_sdk/v2/protocol"
)

// FileEvent is the body of the system.file.event event.
//
// Decoding the fields according to the **producer's actual shape** is critical here: the only
// current publisher is the AppStore file event relay, which POSTs the platform's internal event
// (`{ugn, srcUfi, ufi, inode, action, withAvatar, payload}`) verbatim to the daemon's
// `/api/v1/bus/publish`, and the daemon fans it out to subscribers without re-encoding any field.
// Therefore:
//
//   - **UGN must be decoded**: native is a shared instance that is "installed once and serves
//     multiple spaces", and without the user space an event cannot be routed to the correct space
//     store; guessing from the pool name would mix up spaces (multiple spaces can have pools with
//     the same name, which is exactly why `fs.Table.PoolIn` exists).
//   - **SrcUFI must be decoded**: a rename event must have both the old and the new UFI so the old
//     record can be found and updated.
//   - `actionName` / `receivedAt` are not filled in by the producer; the SDK adds them at decode
//     time (see normalize), otherwise these two fields, whose existence the documentation states,
//     would always be empty strings.
type FileEvent struct {
	// Action is 1=add / 2=delete / 3=rename (matching the platform constants).
	Action int `json:"action"`
	// ActionName is the readable form of action; the SDK fills it in when the producer does not.
	ActionName string `json:"actionName"`
	// UFI is the unified file identifier (the new UFI on rename).
	UFI string `json:"ufi"`
	// SrcUFI is the UFI before a rename; empty for the other actions.
	SrcUFI string `json:"srcUfi,omitempty"`
	// Inode is the file's inode within the pool (carried by the platform's internal event).
	Inode uint64 `json:"inode,omitempty"`
	// WithAvatar indicates that the platform judged this file to have a stand-in/poster at the time.
	WithAvatar bool `json:"withAvatar,omitempty"`
	// UGN is the user space triple the event belongs to; native apps must route by it.
	UGN EventUGN `json:"ugn"`
	// Payload is business data attached by the publisher.
	Payload json.RawMessage `json:"payload,omitempty"`
	// ReceivedAt is the event time (ISO 8601); the SDK fills it in when the producer does not.
	ReceivedAt string `json:"receivedAt"`
}

// EventUGN is the user space triple carried in the event.
type EventUGN struct {
	Username  string `json:"username"`
	GroupName string `json:"groupName"`
	Namespace string `json:"namespace"`
}

// Space returns the event's deployment space (`{user|group}-{namespace}`).
//
// It uses the same rules as the space names in the mount table and appsdk.SpaceFromHeaders, so it
// can be handed directly to `App.Space(name)` for the authorization check and to obtain that
// space's store.
func (e FileEvent) Space() string {
	return appfs.DeploySpace(e.UGN.Username, e.UGN.GroupName, e.UGN.Namespace)
}

// Scope converts the identity triple in the event into the scope of user-scoped capabilities.
//
// Event-driven handlers often need to both persist data (with Space()) and call user-scoped
// capabilities (KV / media info, which need a scope) in the same callback. Without this layer a
// caller easily fills in only username and omits groupName, so an event from a group space gets
// wrongly treated as personal space.
func (e FileEvent) Scope() Scope {
	return Scope{
		Username:  e.UGN.Username,
		GroupName: e.UGN.GroupName,
		Namespace: e.UGN.Namespace,
	}
}

// normalize fills in the readable fields the producer does not provide and makes sure ReceivedAt
// has at least some value.
func (e *FileEvent) normalize(now time.Time) {
	if e.ActionName == "" {
		e.ActionName = actionName(e.Action)
	}
	if e.ReceivedAt == "" {
		e.ReceivedAt = now.UTC().Format(time.RFC3339)
	}
}

func actionName(action int) string {
	switch action {
	case appconst.FileEventAdd:
		return "add"
	case appconst.FileEventDelete:
		return "delete"
	case appconst.FileEventRename:
		return "rename"
	default:
		return ""
	}
}

// Handler handles one event. The returned error is used only for logging: events are a "hint", not
// a transaction.
type Handler func(ctx context.Context, topic string, data json.RawMessage) error

// Subscription is the lifecycle handle for an event subscription.
type Subscription struct {
	cancel context.CancelFunc
	done   chan struct{}
	once   sync.Once
}

// Close stops dispatch (it does not affect the daemon-side subscription state on the underlying
// connection).
func (s *Subscription) Close() {
	if s == nil {
		return
	}
	s.once.Do(func() {
		s.cancel()
		<-s.done
	})
}

// Subscribe subscribes to topics and dispatches events to handler.
//
// Event semantics are at-least-once, ordering is not guaranteed, and pushes during a connection
// interruption are not re-sent: handler must be idempotent and reconcile on its own when strong
// consistency is required.
func Subscribe(ctx context.Context, client *bus.Client, topics []string, handler Handler) (*Subscription, error) {
	if err := client.Subscribe(ctx, topics...); err != nil {
		return nil, err
	}

	subCtx, cancel := context.WithCancel(ctx)
	sub := &Subscription{cancel: cancel, done: make(chan struct{})}

	go func() {
		defer close(sub.done)
		events := client.Events()
		for {
			select {
			case <-subCtx.Done():
				return
			case event, ok := <-events:
				if !ok {
					return
				}
				if handler == nil {
					continue
				}
				_ = handler(subCtx, event.Topic, event.Data)
			}
		}
	}()

	return sub, nil
}

// SubscribeFileEvents subscribes to file events and decodes them into FileEvent.
//
// It subscribes to the base topic protocol.TopicFileEvent, which the daemon expands to the
// caller's own scope; native apps additionally need the manifest to declare storage.user.file,
// otherwise the subscription is rejected.
//
// The event body is decoded according to the actual shape produced by the producer (the AppStore
// file event relay), with UGN / SrcUFI included; see the FileEvent comment for details.
//
// This is the fire-and-forget subscription: events can be lost when the buffer is full or the
// connection drops. For "must not lose a file change" use SubscribeFileEventsReliable.
func SubscribeFileEvents(ctx context.Context, client *bus.Client, handler func(FileEvent)) (*Subscription, error) {
	return Subscribe(ctx, client, []string{protocol.TopicFileEvent}, func(_ context.Context, _ string, data json.RawMessage) error {
		var event FileEvent
		if err := json.Unmarshal(data, &event); err != nil {
			return err
		}
		event.normalize(time.Now())
		if handler != nil {
			handler(event)
		}
		return nil
	})
}

// SubscribeFileEventsReliable subscribes to file events with at-least-once delivery.
//
// It is the acknowledged counterpart of SubscribeFileEvents: the daemon (through the bus ACK
// protocol, backed by a JetStream durable consumer on the OS side) keeps every delivery until
// this client acknowledges it. The handler's return value is the acknowledgement decision:
//
//   - nil: the event was processed (and, if the handler persists, committed); the SDK sends the
//     ACK for this delivery and the daemon drops it.
//   - non-nil: the event was not processed; the SDK does **not** acknowledge it, so the daemon
//     redelivers it (AckWait, 30s on the v2 daemon) until the handler succeeds or the delivery
//     expires (the durable consumer keeps events for at most 1h). The handler must therefore be
//     idempotent: redeliveries, reconnects and a crash between the commit and the ACK all
//     produce duplicates.
//
// Delivery rules the caller can rely on:
//
//   - disconnects do not lose events: the subscription is replayed on reconnect and the daemon
//     redelivers everything that was not acknowledged;
//   - the bounded fire-and-forget event channel (Options.EventBuffer) does not apply here; the
//     reliable path has its own queue, and when it overflows the delivery is dropped without
//     being acknowledged, i.e. it is redelivered later (Client.Stats().ReliableDropped);
//   - an undecodable event is acknowledged and skipped instead of being redelivered forever
//     (a poison message would otherwise rotate for the whole retention window);
//   - ordering is not guaranteed across redeliveries;
//   - Subscription.Close also cancels the daemon-side reliable subscription (best effort), so
//     a stopped consumer does not keep receiving redeliveries it can never acknowledge.
//
// The daemon must support the reliable mode; on an older daemon this returns
// bus.ErrReliableUnsupported instead of silently degrading to the lossy subscription.
func SubscribeFileEventsReliable(ctx context.Context, client *bus.Client, handler func(FileEvent) error) (*Subscription, error) {
	if err := client.SubscribeReliable(ctx, protocol.TopicFileEvent); err != nil {
		return nil, err
	}

	subCtx, cancel := context.WithCancel(ctx)
	sub := &Subscription{cancel: cancel, done: make(chan struct{})}

	go func() {
		defer close(sub.done)
		events := client.ReliableEvents()
		for {
			select {
			case <-subCtx.Done():
				// Stop the daemon-side subscription as well: otherwise the durable consumer
				// keeps redelivering to a consumer that no longer exists, and every delivery
				// is dropped unacknowledged forever. Best effort and off the Close path.
				go func() {
					unsubCtx, unsubCancel := context.WithTimeout(context.Background(), 3*time.Second)
					defer unsubCancel()
					_ = client.Unsubscribe(unsubCtx, protocol.TopicFileEvent)
				}()
				return
			case event, ok := <-events:
				if !ok {
					return
				}
				var fileEvent FileEvent
				if err := json.Unmarshal(event.Data, &fileEvent); err != nil {
					// Poison message: ack and skip. Not acking would make the daemon redeliver
					// this undecodable body until retention expires, blocking nothing but
					// wasting the whole retention window on an event nobody can process.
					_ = client.Ack(event.ID)
					continue
				}
				fileEvent.normalize(time.Now())
				if handler != nil {
					if err := handler(fileEvent); err != nil {
						// No ACK: the daemon redelivers after AckWait. The handler is expected
						// to be idempotent and to surface/log its own failures.
						continue
					}
				}
				_ = client.Ack(event.ID)
			}
		}
	}()

	return sub, nil
}
