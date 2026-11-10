package capability

// Interest listener (shared-only target mode):
//
//	app(SDK v2) --bus call system.interest.listen--> daemon
//	                                              └─ OS /os/interest/listen
//	After receiving listenerId/consumer + nats_url, the app **connects to NATS itself**
//	and pull-Fetches from the shared-stream durable consumer, matching only on matched.listeners.
//
// The daemon selects nats_url for container and host forms (container: bridge gateway; host: 127.0.0.1).
// The SDK never guesses the runtime form.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"

	"github.com/siyouyun-open/siyouyun_sdk/v2/bus"
	"github.com/siyouyun-open/siyouyun_sdk/v2/protocol"
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
	// NATSURL is for tests/special environments only; normally empty and returned by the daemon.
	NATSURL string
	// When FailOnError is true, a handler error is terminal: report failed and ACK,
	// without NAK/redelivery. Default false preserves at-least-once retry semantics.
	FailOnError bool
}

type interestListenRequest struct {
	Dirs        []string `json:"dirs"`
	MediaTypes  []string `json:"mediaTypes,omitempty"`
	ExcludeExts []string `json:"excludeExts,omitempty"`
	Actions     []string `json:"actions,omitempty"`
	QoS         string   `json:"qos,omitempty"`
	TTLSeconds  int      `json:"ttlSec,omitempty"`
}

type interestListenResponse struct {
	ListenerID    string `json:"listenerId"`
	Consumer      string `json:"consumer"`
	Stream        string `json:"stream"`
	Subject       string `json:"subject"`
	NATSURL       string `json:"natsUrl"`
	AppCode       string `json:"appCode"`
	Group         string `json:"group"`
	MaxAckPending int    `json:"maxAckPending"`
	TTLSeconds    int    `json:"ttlSec"`
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
	client     *bus.Client
	listenerID string
	consumer   string
	stream     string
	subject    string
	natsURL    string
	ttl        time.Duration
	nc         *nats.Conn
	js         jetstream.JetStream
	appCode    string
	handler    func(FileEvent) error
	opts       FileEventListenerOptions

	mu          sync.RWMutex
	fetchCancel context.CancelFunc

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
	once   sync.Once
}

// StartFileEventListener requests a listener over the bus, then consumes NATS directly.
func StartFileEventListener(ctx context.Context, client *bus.Client, opts FileEventListenerOptions, handler func(FileEvent) error) (*FileEventListener, error) {
	if client == nil {
		return nil, errors.New("capability: nil bus client")
	}
	if len(opts.Dirs) == 0 {
		return nil, errors.New("capability: dirs is required (root listening is forbidden)")
	}
	ttl := opts.TTL
	if ttl <= 0 {
		ttl = 30 * time.Minute
	}
	opts.TTL = ttl
	resp, err := registerInterest(ctx, client, opts)
	if err != nil {
		return nil, err
	}
	natsURL := opts.NATSURL
	if natsURL == "" {
		natsURL = resp.NATSURL
	}
	if natsURL == "" {
		return nil, errors.New("capability: daemon did not return a nats_url")
	}
	nc, err := nats.Connect(natsURL, nats.Timeout(5*time.Second))
	if err != nil {
		_ = stopInterestListener(client, resp.ListenerID)
		return nil, fmt.Errorf("capability: connect %s: %w", natsURL, err)
	}
	js, err := jetstream.New(nc)
	if err != nil {
		nc.Close()
		_ = stopInterestListener(client, resp.ListenerID)
		return nil, fmt.Errorf("capability: jetstream: %w", err)
	}
	stream, err := js.Stream(ctx, resp.Stream)
	if err != nil {
		nc.Close()
		_ = stopInterestListener(client, resp.ListenerID)
		return nil, fmt.Errorf("capability: stream %s: %w", resp.Stream, err)
	}
	cons, err := stream.Consumer(ctx, resp.Consumer)
	if err != nil {
		nc.Close()
		_ = stopInterestListener(client, resp.ListenerID)
		return nil, fmt.Errorf("capability: consumer %s: %w", resp.Consumer, err)
	}

	l := &FileEventListener{
		client: client, listenerID: resp.ListenerID, consumer: resp.Consumer,
		stream: resp.Stream, subject: resp.Subject, natsURL: natsURL,
		ttl: ttl, nc: nc, js: js, appCode: resp.AppCode, handler: handler, opts: opts,
	}
	l.ctx, l.cancel = context.WithCancel(context.Background())
	fetchCtx, fetchCancel := context.WithCancel(l.ctx)
	l.fetchCancel = fetchCancel
	l.wg.Add(2)
	go func() { defer l.wg.Done(); l.consumeLoop(fetchCtx, cons) }()
	go func() { defer l.wg.Done(); l.renewLoop() }()
	return l, nil
}

func registerInterest(ctx context.Context, client *bus.Client, opts FileEventListenerOptions) (interestListenResponse, error) {
	ttl := opts.TTL
	if ttl <= 0 {
		ttl = 30 * time.Minute
	}
	reqBody := interestListenRequest{
		Dirs:        opts.Dirs,
		MediaTypes:  opts.MediaTypes,
		ExcludeExts: opts.ExcludeExts,
		Actions:     opts.Actions,
		QoS:         opts.QoS,
		TTLSeconds:  int(ttl.Seconds()),
	}
	raw, err := json.Marshal(reqBody)
	if err != nil {
		return interestListenResponse{}, err
	}
	var resp interestListenResponse
	if err := client.CallInto(ctx, &bus.Request{Template: protocol.TemplateInterestListen, Payload: json.RawMessage(raw)}, &resp); err != nil {
		return interestListenResponse{}, err
	}
	if resp.ListenerID == "" || resp.Consumer == "" || resp.Stream == "" {
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

// Consumer returns the durable consumer name on the shared stream.
func (l *FileEventListener) Consumer() string {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return l.consumer
}

// Stream returns the shared stream name.
func (l *FileEventListener) Stream() string {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return l.stream
}

// Close stops Fetch, disconnects NATS, and asks the daemon to delete the listener.
func (l *FileEventListener) Close() {
	if l == nil {
		return
	}
	l.once.Do(func() {
		if l.cancel != nil {
			l.cancel()
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
		for msg := range batch.Messages() {
			l.handle(msg)
		}
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
	ev := FileEvent{
		Action: payload.Action,
		UFI:    payload.UFI, SrcUFI: payload.SrcUFI, Inode: payload.Inode,
		UGN: payload.UGN,
	}
	ev.normalize(time.Now())
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

// recover requests a new listener after a renew failure and switches Fetch to the new consumer.
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
	l.listenerID = resp.ListenerID
	l.consumer = resp.Consumer
	l.stream = resp.Stream
	l.subject = resp.Subject
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
