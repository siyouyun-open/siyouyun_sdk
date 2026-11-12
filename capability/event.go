package capability

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"time"

	"github.com/siyouyun-open/siyouyun_sdk/v2/appconst"
	"github.com/siyouyun-open/siyouyun_sdk/v2/bus"
	appfs "github.com/siyouyun-open/siyouyun_sdk/v2/fs"
	"github.com/siyouyun-open/siyouyun_sdk/v2/protocol"
)

// FileEvent is the event handed to an interest listener, in both transports (NATS and pure bus).
//
// The wire body is the OS interest event
// `{v, eventId, ufi, srcUfi, action, inode, ugn, matched}`: the SDK decodes it, fills in the
// readable fields the producer does not send (ActionName / ReceivedAt) and only then calls the
// application handler. The fields are:
//
//   - **UGN must be kept**: native is a shared instance that is "installed once and serves
//     multiple spaces", and without the user space an event cannot be routed to the correct space
//     store; guessing from the pool name would mix up spaces (multiple spaces can have pools with
//     the same name, which is exactly why `fs.Table.PoolIn` exists).
//   - **SrcUFI must be kept**: a delete/rename event needs the old UFI so the old record can be
//     found and updated.
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

// Handler handles one bus event. The returned error is used only for logging: events are a
// "hint", not a transaction.
type Handler func(ctx context.Context, topic string, data json.RawMessage) error

// Subscription is the lifecycle handle for a bus topic subscription created by Subscribe.
type Subscription struct {
	router *eventRouter
	sub    *eventSubscription
	once   sync.Once
}

// Close stops dispatch and drops the bus subscription when it was the last one on that topic.
func (s *Subscription) Close() {
	if s == nil {
		return
	}
	s.once.Do(func() {
		close(s.sub.closed)
		s.router.remove(s.sub)
	})
}

// Subscribe subscribes to event topics and dispatches matching events to handler.
//
// Event semantics are at-most-once when the bus connection breaks (events are hints, not a
// persistent log), ordering is not guaranteed, and the bounded event channel drops when full:
// the handler must be idempotent and reconcile on its own when strong consistency is required.
//
// All capability-level subscriptions on one bus client share a single event pump: a topic push is
// fanned out to every matching subscription instead of being consumed by whichever goroutine
// happens to read the channel first. Subscribing to a base topic (`system.interest.event`) also
// matches its daemon-expanded form (`system.interest.event.<app_id>`).
func Subscribe(ctx context.Context, client *bus.Client, topics []string, handler Handler) (*Subscription, error) {
	if client == nil {
		return nil, errors.New("capability: nil bus client")
	}
	cleaned, err := normalizeTopics(topics)
	if err != nil {
		return nil, err
	}
	r := routerFor(client)
	sub := &eventSubscription{topics: cleaned, handler: handler, closed: make(chan struct{})}
	if err := r.add(ctx, sub); err != nil {
		return nil, err
	}
	s := &Subscription{router: r, sub: sub}
	// A canceled caller context is equivalent to Close: the subscription must not keep
	// dispatching (and must not keep the bus topic subscribed) after the caller is done.
	go func() {
		select {
		case <-ctx.Done():
			s.Close()
		case <-sub.closed:
		}
	}()
	return s, nil
}

func normalizeTopics(topics []string) (map[string]struct{}, error) {
	cleaned := make(map[string]struct{}, len(topics))
	for _, topic := range topics {
		topic = strings.TrimSpace(topic)
		if topic == "" {
			continue
		}
		cleaned[topic] = struct{}{}
	}
	if len(cleaned) == 0 {
		return nil, errors.New("capability: at least one subscription topic is required")
	}
	return cleaned, nil
}

// ── Shared event pump ──
//
// One bus.Client has one event channel (the daemon pushes every topic onto it), so the capability
// layer must read it exactly once and route the events by topic. Per-subscription readers would
// steal each other's events; with several interest listeners on one app that would silently drop
// events of one listener.

// eventTopology maps a bus client to its event pump. The entry is kept for the client's lifetime
// (an app normally has exactly one): deleting it on the last unsubscribe would race with a
// concurrent Subscribe that already resolved the same router, leaving two pumps reading the single
// event channel.
var eventTopology = struct {
	sync.Mutex
	byClient map[*bus.Client]*eventRouter
}{byClient: make(map[*bus.Client]*eventRouter)}

func routerFor(client *bus.Client) *eventRouter {
	eventTopology.Lock()
	defer eventTopology.Unlock()
	r := eventTopology.byClient[client]
	if r == nil {
		r = &eventRouter{client: client, subs: make(map[*eventSubscription]struct{}), topicRefs: make(map[string]int)}
		eventTopology.byClient[client] = r
	}
	return r
}

type eventRouter struct {
	client *bus.Client

	mu        sync.Mutex
	subs      map[*eventSubscription]struct{}
	topicRefs map[string]int // base topic -> number of subscriptions interested in it
	gen       int
	cancel    context.CancelFunc
}

type eventSubscription struct {
	topics  map[string]struct{}
	handler Handler
	closed  chan struct{}
}

// matches reports whether an expanded topic belongs to this subscription: the daemon expands a
// base topic T into T.<app_id>, so both the exact base topic and its expanded form match.
func (s *eventSubscription) matches(topic string) bool {
	for base := range s.topics {
		if topic == base || strings.HasPrefix(topic, base+".") {
			return true
		}
	}
	return false
}

func (r *eventRouter) add(ctx context.Context, sub *eventSubscription) error {
	toSubscribe := make([]string, 0, len(sub.topics))
	r.mu.Lock()
	for topic := range sub.topics {
		if r.topicRefs[topic] == 0 {
			toSubscribe = append(toSubscribe, topic)
		}
		r.topicRefs[topic]++
	}
	r.mu.Unlock()

	if len(toSubscribe) > 0 {
		if err := r.client.Subscribe(ctx, toSubscribe...); err != nil {
			r.mu.Lock()
			for topic := range sub.topics {
				r.releaseTopicLocked(topic)
			}
			r.mu.Unlock()
			return err
		}
	}

	r.mu.Lock()
	r.subs[sub] = struct{}{}
	r.startPumpLocked()
	r.mu.Unlock()
	return nil
}

func (r *eventRouter) remove(sub *eventSubscription) {
	r.mu.Lock()
	if _, ok := r.subs[sub]; !ok {
		r.mu.Unlock()
		return
	}
	delete(r.subs, sub)
	toUnsubscribe := make([]string, 0, len(sub.topics))
	for topic := range sub.topics {
		if r.releaseTopicLocked(topic) {
			toUnsubscribe = append(toUnsubscribe, topic)
		}
	}
	if len(r.subs) == 0 && r.cancel != nil {
		r.cancel()
		r.cancel = nil
		r.gen++
	}
	r.mu.Unlock()

	if len(toUnsubscribe) > 0 {
		unsubCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_ = r.client.Unsubscribe(unsubCtx, toUnsubscribe...)
	}
}

// releaseTopicLocked drops one reference to topic and reports whether its bus subscription must be
// canceled (the last reference went away).
func (r *eventRouter) releaseTopicLocked(topic string) bool {
	r.topicRefs[topic]--
	if r.topicRefs[topic] > 0 {
		return false
	}
	delete(r.topicRefs, topic)
	return true
}

func (r *eventRouter) startPumpLocked() {
	if r.cancel != nil {
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	r.cancel = cancel
	r.gen++
	go r.run(r.gen, ctx)
}

// run is the single reader of the bus event channel. A generation check makes the old reader of a
// stopped pump stop dispatching even if it is scheduled once more before it observes the cancel.
func (r *eventRouter) run(gen int, ctx context.Context) {
	events := r.client.Events()
	for {
		select {
		case <-ctx.Done():
			return
		case event, ok := <-events:
			if !ok {
				return
			}
			r.dispatch(gen, ctx, event)
		}
	}
}

func (r *eventRouter) dispatch(gen int, ctx context.Context, event protocol.Event) {
	r.mu.Lock()
	if gen != r.gen {
		r.mu.Unlock()
		return
	}
	matched := make([]*eventSubscription, 0, len(r.subs))
	for sub := range r.subs {
		if sub.matches(event.Topic) {
			matched = append(matched, sub)
		}
	}
	r.mu.Unlock()
	// Call handlers outside the lock: a handler may Close its own subscription (and a slow
	// handler must not block sub/unsubscribe bookkeeping).
	for _, sub := range matched {
		if sub.handler == nil {
			continue
		}
		_ = sub.handler(ctx, event.Topic, event.Data)
	}
}
