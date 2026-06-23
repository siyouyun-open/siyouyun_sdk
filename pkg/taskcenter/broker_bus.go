package taskcenter

import (
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/siyouyun-open/siyouyun_sdk/v2/internal/transport"
)

// BusBroker implements MessageBroker via the bus WebSocket transport.
// Used in RUNC mode where NATS is unavailable; all task-center communication
// is routed through the minictl daemon's capability bus.
type BusBroker struct {
	appCode string
	bus     *transport.BusTransport
	subs    map[string]func(msg *Msg)
	mu      sync.Mutex
}

// NewBusBroker creates a BusBroker using the global bus transport.
func NewBusBroker(appCode string) MessageBroker {
	return &BusBroker{
		appCode: appCode,
		bus:     transport.GetBus(),
		subs:    make(map[string]func(msg *Msg)),
	}
}

func (b *BusBroker) Publish(subject string, data []byte) error {
	payload := busPayload{Subject: subject, Data: data}
	_, err := b.bus.Call("system.taskcenter.publish", payload)
	return err
}

func (b *BusBroker) Subscribe(subject string, handler MsgHandler) (Subscription, error) {
	b.mu.Lock()
	b.subs[subject] = handler
	b.mu.Unlock()

	if err := b.bus.SubscribeTopic(subject); err != nil {
		return nil, err
	}

	b.bus.OnTopicEvent(subject, func(topic string, event json.RawMessage) {
		b.mu.Lock()
		h, ok := b.subs[topic]
		b.mu.Unlock()
		if !ok {
			return
		}
		var payload busPayload
		if err := json.Unmarshal(event, &payload); err != nil {
			return
		}
		h(&Msg{
			Subject: topic,
			Data:    payload.Data,
		})
	})

	return &busSub{broker: b, subject: subject}, nil
}

func (b *BusBroker) Request(subject string, data []byte, timeout time.Duration) (*Msg, error) {
	payload := busPayload{Subject: subject, Data: data}
	resp, err := b.bus.Call("system.taskcenter.request", payload)
	if err != nil {
		return nil, err
	}
	var result busPayload
	if err := json.Unmarshal(resp.Data, &result); err != nil {
		return nil, fmt.Errorf("unmarshal bus response: %w", err)
	}
	return &Msg{Subject: subject, Data: result.Data}, nil
}

type busPayload struct {
	Subject string `json:"subject"`
	Data    []byte `json:"data"`
}

type busSub struct {
	broker  *BusBroker
	subject string
}

func (s *busSub) Unsubscribe() error {
	s.broker.mu.Lock()
	delete(s.broker.subs, s.subject)
	s.broker.mu.Unlock()
	return nil
}
