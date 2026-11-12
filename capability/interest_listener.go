package capability

// Interest listener (shared-only target mode) with two transports:
//
//	nats (default): the app consumes the OS shared stream itself
//
//	  app(SDK v2) --bus call system.interest.listen (transport=nats)--> daemon
//	                                                                └─ OS /os/interest/listen
//	  The response carries listenerId/consumer/stream + nats_url; the app **connects to NATS
//	  itself** and pull-Fetches from the shared-stream durable consumer, matching only on
//	  matched.listeners.
//
//	bus (pure bus, no NATS on the app side)
//
//	  app(SDK v2) --bus call system.interest.listen (transport=bus)--> daemon
//	                                                                ├─ OS /os/interest/listen
//	                                                                └─ pump (host NATS -> bus)
//	  The daemon owns the NATS pump and fans the raw OS event out to
//	  system.interest.event.<app_id>; the app only subscribes to the base topic
//	  system.interest.event over the bus. The daemon holds the OS message until the
//	  SDK calls system.interest.ack (success/retry/failed), so handler errors are
//	  still visible to the OS without a local NATS client.
//
// The daemon selects nats_url for container and host forms (container: bridge gateway; host: 127.0.0.1).
// The SDK never guesses the runtime form.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"

	"github.com/siyouyun-open/siyouyun_sdk/v2/bus"
	"github.com/siyouyun-open/siyouyun_sdk/v2/protocol"
)

// Interest listener transports.
const (
	// TransportNATS (default) has the app consume the OS shared stream over NATS directly.
	TransportNATS = "nats"
	// TransportBus is the pure-bus mode: the daemon pumps NATS and fans the events out over the
	// bus, so the app never connects NATS.
	TransportBus = "bus"
)

// FileEventListenerOptions describes one interest listener.
type FileEventListenerOptions struct {
	// Dirs must be at least one level deep; root/wildcard dirs are forbidden (the OS validates again).
	Dirs []string
	// MediaTypes are image / video / image-video / all.
	MediaTypes []string
	// ExcludeExts lists excluded extensions (.tmp / .part).
	ExcludeExts []string
	// Actions are add / delete / rename.
	Actions []string
	// QoS is high / medium / low; defaults to medium.
	QoS string
	// TTL is the listener lease, default 30 minutes; the SDK renews it periodically.
	TTL time.Duration
	// Transport selects how events reach the app: TransportNATS (default) or TransportBus.
	//
	// In bus mode the app must not have a NATS endpoint (container/host alike): the daemon runs
	// the pump and publishes to system.interest.event.<app_id>, which the SDK subscribes to over
	// the bus. Delivery acknowledgement is daemon-side in that mode.
	Transport string
	// NATSURL is for tests/special environments only; normally empty and returned by the daemon.
	// It is ignored in bus mode.
	NATSURL string
	// When FailOnError is true, a handler error is terminal: report failed and ACK,
	// without NAK/redelivery. Default false preserves at-least-once retry semantics.
	// In bus mode a handler error sends status=retry by default, or status=failed
	// when FailOnError is true. The OS-assigned worker limit only applies to NATS mode.
	FailOnError bool
}

type interestListenRequest struct {
	Dirs        []string `json:"dirs"`
	MediaTypes  []string `json:"mediaTypes,omitempty"`
	ExcludeExts []string `json:"excludeExts,omitempty"`
	Actions     []string `json:"actions,omitempty"`
	QoS         string   `json:"qos,omitempty"`
	TTLSeconds  int      `json:"ttlSec,omitempty"`
	// Transport is "nats" (default) or "bus"; the daemon treats an empty value as nats.
	Transport string `json:"transport,omitempty"`
}

type interestListenResponse struct {
	ListenerID     string `json:"listenerId"`
	Consumer       string `json:"consumer"`
	Stream         string `json:"stream"`
	Subject        string `json:"subject"`
	NATSURL        string `json:"natsUrl"`
	AppCode        string `json:"appCode"`
	Group          string `json:"group"`
	MaxAckPending  int    `json:"maxAckPending"`
	TTLSeconds     int    `json:"ttlSec"`
	Workers        int    `json:"workers"`
	ControlSubject string `json:"controlSubject"`
	// Transport is the effective transport reported by the daemon ("nats" / "bus").
	Transport string `json:"transport"`
	// BusTopic is the expanded topic the daemon publishes to in bus mode; informational (the SDK
	// subscribes to the base template).
	BusTopic string `json:"busTopic"`
}

type interestEventPayload struct {
	V       int      `json:"v"`
	EventID string   `json:"eventId"`
	UFI     string   `json:"ufi"`
	SrcUFI  string   `json:"srcUfi"`
	Action  int      `json:"action"`
	Inode   uint64   `json:"inode"`
	UGN     EventUGN `json:"ugn"`
	Matched struct {
		Apps      []string `json:"apps"`
		Listeners []string `json:"listeners"`
	} `json:"matched"`
}

// FileEventListener is a started listener handle.
type FileEventListener struct {
	client      *bus.Client
	listenerID  string
	consumer    string
	stream      string
	subject     string
	natsURL     string
	transport   string
	ttl         time.Duration
	nc          *nats.Conn
	js          jetstream.JetStream
	appCode     string
	handler     func(FileEvent) error
	opts        FileEventListenerOptions
	workerLimit atomic.Int64
	controlSub  *nats.Subscription
	busSub      *Subscription

	mu          sync.RWMutex
	fetchCancel context.CancelFunc

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
	once   sync.Once
}

// StartFileEventListener requests a listener over the bus, then starts consuming events using the
// requested transport: NATS (connect to the OS shared stream consumer) or bus (the daemon fans the
// events out over the bus).
func StartFileEventListener(ctx context.Context, client *bus.Client, opts FileEventListenerOptions, handler func(FileEvent) error) (*FileEventListener, error) {
	if client == nil {
		return nil, errors.New("capability: nil bus client")
	}
	if len(opts.Dirs) == 0 {
		return nil, errors.New("capability: dirs is required (root listening is forbidden)")
	}
	transport, err := normalizeInterestTransport(opts.Transport)
	if err != nil {
		return nil, err
	}
	opts.Transport = transport
	ttl := opts.TTL
	if ttl <= 0 {
		ttl = 30 * time.Minute
	}
	opts.TTL = ttl
	resp, err := registerInterest(ctx, client, opts)
	if err != nil {
		return nil, err
	}

	l := &FileEventListener{
		client: client, listenerID: resp.ListenerID, consumer: resp.Consumer,
		stream: resp.Stream, subject: resp.Subject, natsURL: resp.NATSURL,
		transport: transport, ttl: ttl, appCode: resp.AppCode, handler: handler, opts: opts,
	}
	l.ctx, l.cancel = context.WithCancel(context.Background())
	if transport == TransportBus {
		return l.startBusMode()
	}
	return l.startNATSMode(ctx, resp)
}

// startBusMode subscribes to the daemon's pure-bus interest topic and dispatches events to the
// handler. The app never connects NATS in this mode. Events are dispatched one at a time in arrival
// order (there is no local worker pool: the daemon has already taken delivery of the OS message).
func (l *FileEventListener) startBusMode() (*FileEventListener, error) {
	sub, err := Subscribe(l.ctx, l.client, []string{protocol.TopicInterestEvent}, func(_ context.Context, _ string, data json.RawMessage) error {
		l.handleBusEvent(data)
		return nil
	})
	if err != nil {
		l.cancel()
		_ = stopInterestListener(l.client, l.ListenerID())
		return nil, fmt.Errorf("capability: subscribe %s: %w", protocol.TopicInterestEvent, err)
	}
	l.busSub = sub
	l.wg.Add(1)
	go func() { defer l.wg.Done(); l.renewLoop() }()
	return l, nil
}

func (l *FileEventListener) startNATSMode(callCtx context.Context, resp interestListenResponse) (*FileEventListener, error) {
	natsURL := l.opts.NATSURL
	if natsURL == "" {
		natsURL = resp.NATSURL
	}
	if natsURL == "" {
		l.cancel()
		_ = stopInterestListener(l.client, l.ListenerID())
		return nil, errors.New("capability: daemon did not return a nats_url")
	}
	nc, err := nats.Connect(natsURL, nats.Timeout(5*time.Second))
	if err != nil {
		l.cancel()
		_ = stopInterestListener(l.client, l.ListenerID())
		return nil, fmt.Errorf("capability: connect %s: %w", natsURL, err)
	}
	l.nc = nc
	l.natsURL = natsURL
	js, err := jetstream.New(nc)
	if err != nil {
		nc.Close()
		l.cancel()
		_ = stopInterestListener(l.client, l.ListenerID())
		return nil, fmt.Errorf("capability: jetstream: %w", err)
	}
	l.js = js
	stream, err := js.Stream(callCtx, resp.Stream)
	if err != nil {
		nc.Close()
		l.cancel()
		_ = stopInterestListener(l.client, l.ListenerID())
		return nil, fmt.Errorf("capability: stream %s: %w", resp.Stream, err)
	}
	cons, err := stream.Consumer(callCtx, resp.Consumer)
	if err != nil {
		nc.Close()
		l.cancel()
		_ = stopInterestListener(l.client, l.ListenerID())
		return nil, fmt.Errorf("capability: consumer %s: %w", resp.Consumer, err)
	}

	initialWorkers := resp.Workers
	if initialWorkers <= 0 {
		initialWorkers = 1
	}
	l.workerLimit.Store(int64(normalizeInterestWorkers(initialWorkers)))
	control := resp.ControlSubject
	if control == "" {
		control = "os.interest.control." + resp.ListenerID
	}
	l.controlSub, _ = nc.Subscribe(control, func(msg *nats.Msg) {
		var ctl struct {
			Workers int `json:"workers"`
		}
		if json.Unmarshal(msg.Data, &ctl) == nil && ctl.Workers > 0 {
			l.workerLimit.Store(int64(normalizeInterestWorkers(ctl.Workers)))
			fmt.Printf("[interest] worker limit updated listener=%s workers=%d\n", l.listenerID, ctl.Workers)
		}
	})

	fetchCtx, fetchCancel := context.WithCancel(l.ctx)
	l.fetchCancel = fetchCancel
	l.wg.Add(2)
	go func() { defer l.wg.Done(); l.consumeLoop(fetchCtx, cons) }()
	go func() { defer l.wg.Done(); l.renewLoop() }()
	return l, nil
}

func normalizeInterestTransport(raw string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "", TransportNATS:
		return TransportNATS, nil
	case TransportBus:
		return TransportBus, nil
	default:
		return "", fmt.Errorf("capability: unsupported interest transport %q (want %q or %q)", raw, TransportNATS, TransportBus)
	}
}

func registerInterest(ctx context.Context, client *bus.Client, opts FileEventListenerOptions) (interestListenResponse, error) {
	ttl := opts.TTL
	if ttl <= 0 {
		ttl = 30 * time.Minute
	}
	transport, err := normalizeInterestTransport(opts.Transport)
	if err != nil {
		return interestListenResponse{}, err
	}
	reqBody := interestListenRequest{
		Dirs:        opts.Dirs,
		MediaTypes:  opts.MediaTypes,
		ExcludeExts: opts.ExcludeExts,
		Actions:     opts.Actions,
		QoS:         opts.QoS,
		TTLSeconds:  int(ttl.Seconds()),
		Transport:   transport,
	}
	raw, err := json.Marshal(reqBody)
	if err != nil {
		return interestListenResponse{}, err
	}
	var resp interestListenResponse
	if err := client.CallInto(ctx, &bus.Request{Template: protocol.TemplateInterestListen, Payload: json.RawMessage(raw)}, &resp); err != nil {
		return interestListenResponse{}, err
	}
	if resp.ListenerID == "" {
		return interestListenResponse{}, fmt.Errorf("capability: invalid interest listener response: %+v", resp)
	}
	if transport == TransportBus {
		// No silent degradation to NATS: a daemon without pure-bus support must fail the listener
		// instead of handing back a NATS consumer the app cannot use.
		if resp.Transport != TransportBus {
			return interestListenResponse{}, fmt.Errorf("capability: daemon did not honor transport=%s (got %q)", TransportBus, resp.Transport)
		}
	} else if resp.Consumer == "" || resp.Stream == "" {
		return interestListenResponse{}, fmt.Errorf("capability: invalid interest listener response: %+v", resp)
	}
	return resp, nil
}

// ListenerID returns the listenerId issued by the OS.
func (l *FileEventListener) ListenerID() string {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return l.listenerID
}

// Transport returns the effective transport ("nats" or "bus").
func (l *FileEventListener) Transport() string {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return l.transport
}

// Consumer returns the durable consumer name on the shared stream. It is informational in bus
// mode: the daemon owns that consumer and the app does not use it.
func (l *FileEventListener) Consumer() string {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return l.consumer
}

// Stream returns the shared stream name. It is informational in bus mode: the daemon owns that
// stream/consumer and the app does not use it.
func (l *FileEventListener) Stream() string {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return l.stream
}

// InterestListenerInfo is the daemon-side metadata for one interest listener.
type InterestListenerInfo struct {
	ListenerID     string   `json:"listenerId"`
	ChannelID      string   `json:"channelId,omitempty"`
	AppCode        string   `json:"appCode"`
	AppID          string   `json:"appId,omitempty"`
	Namespace      string   `json:"namespace"`
	Transport      string   `json:"transport"`
	BusTopic       string   `json:"busTopic,omitempty"`
	Dirs           []string `json:"dirs"`
	Actions        []string `json:"actions,omitempty"`
	ExcludeExts    []string `json:"excludeExts,omitempty"`
	MediaTypes     []string `json:"mediaTypes,omitempty"`
	QoS            string   `json:"qos,omitempty"`
	TTLSeconds     int      `json:"ttlSec,omitempty"`
	Workers        int      `json:"workers,omitempty"`
	ControlSubject string   `json:"controlSubject,omitempty"`
	Stream         string   `json:"stream,omitempty"`
	Consumer       string   `json:"consumer,omitempty"`
	CreatedAt      int64    `json:"createdAt,omitempty"`
	UpdatedAt      int64    `json:"updatedAt,omitempty"`
}

// ListFileEventListeners lists the caller's own interest listeners.
func ListFileEventListeners(ctx context.Context, client *bus.Client) ([]InterestListenerInfo, error) {
	if client == nil {
		return nil, errors.New("capability: nil bus client")
	}
	var resp struct {
		Items []InterestListenerInfo `json:"items"`
		Total int                    `json:"total"`
	}
	if err := client.CallInto(ctx, &bus.Request{Template: protocol.TemplateInterestList, Payload: json.RawMessage(`{}`)}, &resp); err != nil {
		return nil, err
	}
	return resp.Items, nil
}

// GetFileEventListener reads one of the caller's own interest listeners.
func GetFileEventListener(ctx context.Context, client *bus.Client, listenerID string) (InterestListenerInfo, error) {
	if client == nil {
		return InterestListenerInfo{}, errors.New("capability: nil bus client")
	}
	body, _ := json.Marshal(map[string]string{"listenerId": listenerID})
	var info InterestListenerInfo
	if err := client.CallInto(ctx, &bus.Request{Template: protocol.TemplateInterestGet, Payload: body}, &info); err != nil {
		return InterestListenerInfo{}, err
	}
	return info, nil
}

// Update changes the listener filters without changing its listenerId. Both transports use the
// same daemon call; NATS consumers and the pure-bus pump remain attached to the same listener.
func (l *FileEventListener) Update(ctx context.Context, opts FileEventListenerOptions) error {
	if l == nil || l.client == nil {
		return errors.New("capability: nil listener")
	}
	body, err := json.Marshal(map[string]any{
		"listenerId":  l.ListenerID(),
		"dirs":        opts.Dirs,
		"mediaTypes":  opts.MediaTypes,
		"actions":     opts.Actions,
		"excludeExts": opts.ExcludeExts,
	})
	if err != nil {
		return err
	}
	var info InterestListenerInfo
	if err := l.client.CallInto(ctx, &bus.Request{Template: protocol.TemplateInterestUpdate, Payload: body}, &info); err != nil {
		return err
	}
	l.mu.Lock()
	l.opts.Dirs = append([]string(nil), opts.Dirs...)
	l.opts.MediaTypes = append([]string(nil), opts.MediaTypes...)
	l.opts.Actions = append([]string(nil), opts.Actions...)
	l.opts.ExcludeExts = append([]string(nil), opts.ExcludeExts...)
	l.mu.Unlock()
	return nil
}

// Close stops consuming, drops the bus subscription (bus mode) or disconnects NATS (NATS mode),
// and asks the daemon to delete the listener.
func (l *FileEventListener) Close() {
	if l == nil {
		return
	}
	l.once.Do(func() {
		if l.cancel != nil {
			l.cancel()
		}
		if l.busSub != nil {
			l.busSub.Close()
		}
		if l.nc != nil {
			l.nc.Close()
		}
		l.wg.Wait()
		if l.client != nil {
			_ = stopInterestListener(l.client, l.ListenerID())
		}
	})
}

func (l *FileEventListener) consumeLoop(ctx context.Context, cons jetstream.Consumer) {
	for {
		if ctx.Err() != nil {
			return
		}
		batch, err := cons.Fetch(16, jetstream.FetchMaxWait(500*time.Millisecond))
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			time.Sleep(50 * time.Millisecond)
			continue
		}
		workers := int(l.workerLimit.Load())
		if workers < 1 {
			workers = 1
		}
		sem := make(chan struct{}, workers)
		var wg sync.WaitGroup
		for msg := range batch.Messages() {
			wg.Add(1)
			sem <- struct{}{}
			go func(msg jetstream.Msg) {
				defer wg.Done()
				defer func() { <-sem }()
				l.handle(msg)
			}(msg)
		}
		wg.Wait()
	}
}

func (l *FileEventListener) handle(msg jetstream.Msg) {
	var payload interestEventPayload
	if err := json.Unmarshal(msg.Data(), &payload); err != nil {
		_ = msg.Ack()
		return
	}
	if !containsListenerID(payload.Matched.Listeners, l.ListenerID()) {
		_ = msg.Ack()
		return
	}
	ev := interestFileEvent(payload)
	if l.handler != nil {
		if err := l.handler(ev); err != nil {
			if l.opts.FailOnError {
				l.reportResult(payload, "failed", err.Error())
				_ = msg.Ack()
				return
			}
			l.reportResult(payload, "retry", err.Error())
			_ = msg.Nak()
			return
		}
	}
	l.reportResult(payload, "success", "")
	_ = msg.Ack()
}

// handleBusEvent decodes one pure-bus interest event and dispatches it to the handler.
//
// The daemon holds the OS message until this call. The SDK reports the handler result
// through system.interest.ack: success ACKs, retry NAKs for redelivery, and failed
// settles a terminal error.
func (l *FileEventListener) handleBusEvent(data []byte) {
	var payload interestEventPayload
	if err := json.Unmarshal(data, &payload); err != nil {
		return
	}
	if !containsListenerID(payload.Matched.Listeners, l.ListenerID()) {
		return
	}
	status := "success"
	message := ""
	if l.handler != nil {
		if err := l.handler(interestFileEvent(payload)); err != nil {
			message = err.Error()
			if l.opts.FailOnError {
				status = "failed"
			} else {
				status = "retry"
			}
		}
	}
	l.ackBusEvent(payload.EventID, status, message)
}

// ackBusEvent reports the handler outcome to the daemon in pure-bus mode.
// The daemon holds the OS message until this call: success/failed ACK it; retry
// NAKs it for redelivery.
func (l *FileEventListener) ackBusEvent(eventID, status, message string) {
	if l == nil || l.client == nil || eventID == "" {
		return
	}
	body, err := json.Marshal(map[string]any{
		"listenerId": l.ListenerID(), "eventId": eventID,
		"status": status, "message": message,
	})
	if err != nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = l.client.CallInto(ctx, &bus.Request{Template: protocol.TemplateInterestAck, Payload: json.RawMessage(body)}, &struct{}{})
}

// interestFileEvent converts the wire payload into the public event type (filling the fields the
// producer does not send).
func interestFileEvent(payload interestEventPayload) FileEvent {
	ev := FileEvent{
		Action: payload.Action,
		UFI:    payload.UFI, SrcUFI: payload.SrcUFI, Inode: payload.Inode,
		UGN: payload.UGN,
	}
	ev.normalize(time.Now())
	return ev
}

const (
	// interestResultSubject is the fixed subject on which the OS settles interest consumption results.
	interestResultSubject = "os.interest.result"
	// interestResultPublishTimeout bounds JetStream result publishing: reporting runs in the consume callback,
	// so a slow publish must not stall the event stream. Local NATS JS ack is sub-ms; 500ms is only a guard;
	// during the fallback window (old OS without a result stream) each event waits at most this long.
	interestResultPublishTimeout = 500 * time.Millisecond
)

func (l *FileEventListener) reportResult(payload interestEventPayload, status, message string) {
	if l == nil || l.nc == nil || payload.EventID == "" {
		return
	}
	body, _ := json.Marshal(map[string]any{
		"eventId": payload.EventID, "appCode": l.appCode, "listenerId": l.ListenerID(),
		"status": status, "message": message, "ugn": payload.UGN,
	})
	l.publishResult(body)
}

// publishResult prefers JetStream durable publish: the OS result stream persists, so process
// restarts or disconnects can recover. When JetStream is unavailable (no result stream, publish
// timeout, non-JetStream deployment), fall back to core NATS. Fields and subject are unchanged.
func (l *FileEventListener) publishResult(body []byte) {
	if l.nc == nil {
		return
	}
	if l.js != nil {
		ctx, cancel := context.WithTimeout(context.Background(), interestResultPublishTimeout)
		_, err := l.js.Publish(ctx, interestResultSubject, body)
		cancel()
		if err == nil {
			return
		}
	}
	_ = l.nc.Publish(interestResultSubject, body)
}

func (l *FileEventListener) renewLoop() {
	ticker := time.NewTicker(l.ttl / 3)
	defer ticker.Stop()
	for {
		select {
		case <-l.ctx.Done():
			return
		case <-ticker.C:
			body, _ := json.Marshal(map[string]any{
				"listenerId": l.ListenerID(),
				"ttlSec":     int(l.ttl.Seconds()),
			})
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			err := l.client.CallInto(ctx, &bus.Request{Template: protocol.TemplateInterestRenew, Payload: json.RawMessage(body)}, &struct{}{})
			cancel()
			if err != nil {
				// After an OS restart/lease expiry the old listener is gone: listen again
				// instead of spinning forever with an ID that can never renew.
				l.recover()
			}
		}
	}
}

// recover requests a new listener after a renew failure and switches Fetch to the new consumer
// (NATS mode). In bus mode only the listener id changes: the app-scoped bus subscription keeps
// receiving the events of the recovered listener.
func (l *FileEventListener) recover() {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.ctx.Err() != nil {
		return
	}
	resp, err := registerInterest(l.ctx, l.client, l.opts)
	if err != nil {
		return
	}
	l.listenerID = resp.ListenerID
	l.consumer = resp.Consumer
	l.stream = resp.Stream
	l.subject = resp.Subject
	if l.transport == TransportBus {
		return
	}
	stream, err := l.js.Stream(l.ctx, resp.Stream)
	if err != nil {
		return
	}
	cons, err := stream.Consumer(l.ctx, resp.Consumer)
	if err != nil {
		return
	}
	if l.fetchCancel != nil {
		l.fetchCancel()
	}
	fetchCtx, fetchCancel := context.WithCancel(l.ctx)
	l.fetchCancel = fetchCancel
	l.wg.Add(1)
	go func() { defer l.wg.Done(); l.consumeLoop(fetchCtx, cons) }()
}

func stopInterestListener(client *bus.Client, listenerID string) error {
	body, _ := json.Marshal(map[string]any{"listenerId": listenerID})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return client.CallInto(ctx, &bus.Request{Template: protocol.TemplateInterestStop, Payload: json.RawMessage(body)}, &struct{}{})
}

func containsListenerID(items []string, want string) bool {
	for _, s := range items {
		if s == want {
			return true
		}
	}
	return false
}

func normalizeInterestWorkers(n int) int {
	if n <= 1 {
		return 1
	}
	if n > 64 {
		return 64
	}
	return n
}
