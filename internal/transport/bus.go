package transport

import (
	"encoding/json"
	"fmt"
	"net/http"
	"runtime/debug"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"

	"github.com/siyouyun-open/siyouyun_sdk/v2/pkg/apiclient"
	sdkdto "github.com/siyouyun-open/siyouyun_sdk/v2/pkg/dto"
	sdklog "github.com/siyouyun-open/siyouyun_sdk/v2/pkg/log"
	"github.com/siyouyun-open/siyouyun_sdk/v2/pkg/restjson"
	"github.com/siyouyun-open/siyouyun_sdk/v2/pkg/utils"
)

// BusTransport is the RUNC-mode transport that replaces NATS + REST with a single
// WebSocket connection to the minictl daemon's capability bus.
type BusTransport struct {
	conn    *websocket.Conn
	appCode string
	mu      sync.Mutex
	done    chan struct{}
	reqID   atomic.Int64

	pending     map[string]chan busResponse
	subHandlers map[string]func(eventCode string, fe *sdkdto.FileEvent)
	msgHandlers map[string]func(topic string, event json.RawMessage)
	closeOnce   sync.Once
}

type busMessage struct {
	Type     string            `json:"type"`
	ID       string            `json:"id"`
	Template string            `json:"template,omitempty"`
	Payload  json.RawMessage   `json:"payload,omitempty"`
	Headers  map[string]string `json:"headers,omitempty"`
	Topics   []string          `json:"topics,omitempty"`
}

type busResponse struct {
	Type  string          `json:"type"`
	ID    string          `json:"id"`
	Code  string          `json:"code,omitempty"`
	Data  json.RawMessage `json:"data,omitempty"`
	Topic string          `json:"topic,omitempty"`
	Event json.RawMessage `json:"event,omitempty"`
	Error string          `json:"error,omitempty"`
}

type busCallResult struct {
	Code string
	Data json.RawMessage
}

var (
	globalBus     *BusTransport
	globalBusOnce sync.Once
)

// GetBus returns the global BusTransport singleton.
func GetBus() *BusTransport { return globalBus }

// InitBus initializes the global bus transport.
func InitBus(appCode string) error {
	var initErr error
	globalBusOnce.Do(func() {
		apiclient.SetBusFunc(func(method, path string, query map[string]string, body any) ([]byte, error) {
			if globalBus == nil {
				return nil, fmt.Errorf("bus transport not initialized")
			}
			template := RestPathToTemplate(method, path)
			if template == "" {
				return nil, fmt.Errorf("no bus template for %s %s", method, path)
			}
			payload := make(map[string]any)
			if body != nil {
				if raw, err := json.Marshal(body); err == nil {
					json.Unmarshal(raw, &payload)
				}
			}
			for k, v := range query {
				payload[k] = v
			}
			var wrapper restjson.Response[json.RawMessage]
			if err := globalBus.CallJSON(template, payload, &wrapper); err != nil {
				return nil, err
			}
			if wrapper.Code != "A00200" {
				return nil, fmt.Errorf("bus call %s returned %s", template, wrapper.Code)
			}
			if wrapper.Data != nil {
				return *wrapper.Data, nil
			}
			return nil, nil
		})

		globalBus = &BusTransport{
			appCode:     appCode,
			done:        make(chan struct{}),
			pending:     make(map[string]chan busResponse),
			subHandlers: make(map[string]func(eventCode string, fe *sdkdto.FileEvent)),
			msgHandlers: make(map[string]func(topic string, event json.RawMessage)),
		}

		ip := utils.GetIPByEnv()
		url := fmt.Sprintf("ws://%s:9091/api/bus/ws?appCode=%s", ip, appCode)

		dialer := websocket.DefaultDialer
		dialer.HandshakeTimeout = 10 * time.Second

		conn, _, err := dialer.Dial(url, http.Header{})
		if err != nil {
			initErr = fmt.Errorf("bus WS dial failed: %w", err)
			return
		}
		globalBus.conn = conn
		go globalBus.readLoop()
	})
	return initErr
}

// NewBus returns an existing singleton or creates one.
func NewBus(appCode string) (*BusTransport, error) {
	if globalBus != nil && globalBus.appCode == appCode {
		return globalBus, nil
	}
	if err := InitBus(appCode); err != nil {
		return nil, err
	}
	return globalBus, nil
}

// CloseBus shuts down the global bus connection.
func CloseBus() {
	if globalBus != nil {
		globalBus.Close()
	}
}

// ── EventTransport implementation ──

func (t *BusTransport) Subscribe(appCode string, opts map[string]sdkdto.PreferOptions) error {
	topic := fmt.Sprintf("system.file.event.%s", appCode)
	return t.send(busMessage{Type: "subscribe", ID: t.nextID(), Topics: []string{topic}})
}

func (t *BusTransport) OnEvent(handler func(eventCode string, fe *sdkdto.FileEvent)) {
	t.mu.Lock()
	t.subHandlers[fmt.Sprintf("system.file.event.%s", t.appCode)] = handler
	t.mu.Unlock()
}

// SubscribeTopic subscribes to an arbitrary topic.
func (t *BusTransport) SubscribeTopic(topic string) error {
	return t.send(busMessage{Type: "subscribe", ID: t.nextID(), Topics: []string{topic}})
}

// OnTopicEvent registers a callback for generic topic events.
func (t *BusTransport) OnTopicEvent(topic string, handler func(topic string, event json.RawMessage)) {
	t.mu.Lock()
	t.msgHandlers[topic] = handler
	t.mu.Unlock()
}

// Call performs a synchronous RPC call through the bus.
func (t *BusTransport) Call(template string, payload any) (*busCallResult, error) {
	var rawPayload json.RawMessage
	if payload != nil {
		data, err := json.Marshal(payload)
		if err != nil {
			return nil, fmt.Errorf("marshal payload: %w", err)
		}
		rawPayload = data
	}

	id := t.nextID()
	respCh := make(chan busResponse, 1)

	t.mu.Lock()
	t.pending[id] = respCh
	t.mu.Unlock()

	defer func() {
		t.mu.Lock()
		delete(t.pending, id)
		t.mu.Unlock()
	}()

	if err := t.send(busMessage{Type: "call", ID: id, Template: template, Payload: rawPayload}); err != nil {
		return nil, fmt.Errorf("send call: %w", err)
	}

	select {
	case resp := <-respCh:
		if resp.Type == "error" || resp.Error != "" {
			return nil, fmt.Errorf("call %s failed: %s", template, resp.Error)
		}
		return &busCallResult{Code: resp.Code, Data: resp.Data}, nil
	case <-t.done:
		return nil, fmt.Errorf("transport closed")
	case <-time.After(30 * time.Second):
		return nil, fmt.Errorf("call %s timeout", template)
	}
}

// CallJSON unmarshals the response into result.
func (t *BusTransport) CallJSON(template string, payload any, result any) error {
	resp, err := t.Call(template, payload)
	if err != nil {
		return err
	}
	if result != nil && len(resp.Data) > 0 {
		return json.Unmarshal(resp.Data, result)
	}
	return nil
}

// GetAppInfo retrieves app registration info via the bus.
func (t *BusTransport) GetAppInfo() (*sdkdto.AppRegisterInfo, error) {
	var info sdkdto.AppRegisterInfo
	if err := t.CallJSON("system.os.info", nil, &info); err != nil {
		return nil, fmt.Errorf("get app info: %w", err)
	}
	return &info, nil
}

// Close shuts down this transport.
func (t *BusTransport) Close() error {
	t.closeOnce.Do(func() {
		close(t.done)
		if t.conn != nil {
			t.conn.Close()
		}
	})
	return nil
}

// ── internals ──

func (t *BusTransport) nextID() string { return fmt.Sprintf("%d", t.reqID.Add(1)) }

func (t *BusTransport) send(msg busMessage) error {
	if t.conn == nil {
		return fmt.Errorf("bus transport not connected")
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.conn.WriteJSON(msg)
}

func (t *BusTransport) readLoop() {
	defer func() {
		if r := recover(); r != nil {
			sdklog.Logger.Errorf("BusTransport readLoop panic: %v\nStack trace:\n%s", r, debug.Stack())
		}
	}()

	pingTicker := time.NewTicker(50 * time.Second)
	defer pingTicker.Stop()

	go func() {
		for {
			select {
			case <-pingTicker.C:
				t.send(busMessage{Type: "ping", ID: t.nextID()})
			case <-t.done:
				return
			}
		}
	}()

	for {
		select {
		case <-t.done:
			return
		default:
		}

		var resp busResponse
		if err := t.conn.ReadJSON(&resp); err != nil {
			select {
			case <-t.done:
				return
			default:
				sdklog.Logger.Errorf("BusTransport read error: %v", err)
				return
			}
		}

		switch resp.Type {
		case "response":
			t.handleResponse(resp)
		case "event":
			t.handleEvent(resp)
		case "error":
			t.handleResponse(resp)
		}
	}
}

func (t *BusTransport) handleResponse(resp busResponse) {
	t.mu.Lock()
	ch, ok := t.pending[resp.ID]
	t.mu.Unlock()
	if ok {
		select {
		case ch <- resp:
		default:
		}
	}
}

func (t *BusTransport) handleEvent(resp busResponse) {
	t.mu.Lock()
	fh := t.subHandlers[resp.Topic]
	mh := t.msgHandlers[resp.Topic]
	t.mu.Unlock()

	if fh != nil {
		var fe sdkdto.FileEvent
		if err := json.Unmarshal(resp.Event, &fe); err != nil {
			sdklog.Logger.Errorf("BusTransport unmarshal FileEvent: %v", err)
			return
		}
		eventCode := fmt.Sprintf("%d", fe.Action)
		go safeCall(func() { fh(eventCode, &fe) })
		return
	}
	if mh != nil {
		go safeCall(func() { mh(resp.Topic, resp.Event) })
	}
}

func safeCall(fn func()) {
	defer func() {
		if r := recover(); r != nil {
			sdklog.Logger.Errorf("BusTransport handler panic: %v\nStack trace:\n%s", r, debug.Stack())
		}
	}()
	fn()
}
