package ability

import (
	"encoding/json"
	"fmt"
	"sync"

	"github.com/nats-io/nats.go"

	"github.com/siyouyun-open/siyouyun_sdk/v2/internal/gateway"
	"github.com/siyouyun-open/siyouyun_sdk/v2/internal/transport"
	sdklog "github.com/siyouyun-open/siyouyun_sdk/v2/pkg/log"
	"github.com/siyouyun-open/siyouyun_sdk/v2/pkg/utils"
)

type handler func(ugn *utils.UserGroupNamespace, content, uuid string) (reply bool, replyContent string, replyToUUID bool)

type Message struct {
	appCode  *string
	nc       *nats.Conn
	mu       sync.Mutex
	handlers map[string]handler
}

// NewMessage creates a NATS-based message bot (host mode).
func NewMessage(appCode *string, nc *nats.Conn) *Message {
	m := &Message{
		appCode:  appCode,
		nc:       nc,
		mu:       sync.Mutex{},
		handlers: make(map[string]handler),
	}
	m.enableListener()
	return m
}

func (m *Message) Name() string {
	return "Message"
}

func (m *Message) IsReady() bool {
	return true
}

func (m *Message) Close() {
}

// SendMsg sends a message to the user.
func (m *Message) SendMsg(ugn *utils.UserGroupNamespace, content, replyUUID string) error {
	return gateway.SendMessage(ugn, *m.appCode, content, replyUUID)
}

// AddHandler adds a message bot handler.
func (m *Message) AddHandler(desc string, triggerPhrasePerl string, h func(ugn *utils.UserGroupNamespace, content, uuid string) (reply bool, replyContent string, replyToUUID bool)) {
	if desc == "" {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.handlers[desc]; !ok {
		sdklog.Logger.Infof("added message bot handler: [%v]", desc)
		m.handlers[desc] = h
	}
}

// MessageEvent is the structure passed in message events.
type MessageEvent struct {
	UGN         utils.UserGroupNamespace `json:"ugn"`
	UUID        string                   `json:"uuid"`
	Content     string                   `json:"content"`
	SessionId   string                   `json:"sessionId"`
	SendByAdmin bool                     `json:"sendByAdmin"`
}

// NewMessageBus creates a bus-based message bot (RUNC mode).
func NewMessageBus(appCode *string) *Message {
	m := &Message{
		appCode:  appCode,
		handlers: make(map[string]handler),
	}
	m.enableBusListener(appCode)
	return m
}

// enableBusListener subscribes to message events via the bus pub/sub.
func (m *Message) enableBusListener(appCode *string) {
	bus := transport.GetBus()
	if bus == nil {
		return
	}
	topic := fmt.Sprintf("system.msg.%s", *appCode)
	bus.SubscribeTopic(topic)
	bus.OnTopicEvent(topic, func(topic string, event json.RawMessage) {
		var mes []MessageEvent
		if err := json.Unmarshal(event, &mes); err != nil {
			return
		}
		for i := range mes {
			ugn := utils.NewUserGroupNamespace(mes[i].UGN.Username, mes[i].UGN.GroupName, mes[i].UGN.Namespace)
			if !mes[i].SendByAdmin {
				for _, h := range m.handlers {
					reply, content, replyToUUID := h(ugn, mes[i].Content, mes[i].UUID)
					if reply {
						var replyUUID string
						if replyToUUID {
							replyUUID = mes[i].UUID
						}
						m.SendMsg(ugn, content, replyUUID)
					}
				}
			}
		}
	})
}

// enableListener starts the NATS listener for incoming messages (host mode).
func (m *Message) enableListener() {
	robotCode := *m.appCode + "_msg"
	go func() {
		sdklog.Logger.Infof("start ListenBizMsg at:%v", robotCode)
		_, err := m.nc.Subscribe(robotCode, func(msg *nats.Msg) {
			var mes []MessageEvent
			defer func() {
				if err := recover(); err != nil {
					sdklog.Logger.Infof("nats panic:[%v]-[%v]", err, mes)
				}
			}()
			err := json.Unmarshal(msg.Data, &mes)
			if err != nil {
				return
			}
			for i := range mes {
				ugn := utils.NewUserGroupNamespace(mes[i].UGN.Username, mes[i].UGN.GroupName, mes[i].UGN.Namespace)
				if !mes[i].SendByAdmin {
					for i2 := range m.handlers {
						reply, content, replyToUUID := m.handlers[i2](ugn, mes[i].Content, mes[i].UUID)
						if reply {
							var replyUUID string
							if replyToUUID {
								replyUUID = mes[i].UUID
							}
							err = gateway.SendMessage(ugn, *m.appCode, content, replyUUID)
							if err != nil {
								continue
							}
						}
					}
				}
			}
		})
		if err != nil {
			sdklog.Logger.Errorf("enableListener subscribe err: %v", err)
		}
	}()
}
