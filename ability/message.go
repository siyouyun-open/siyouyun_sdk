package ability

import (
	"encoding/json"
	"sync"

	"github.com/nats-io/nats.go"

	"github.com/siyouyun-open/siyouyun_sdk/internal/gateway"
	sdklog "github.com/siyouyun-open/siyouyun_sdk/pkg/log"
	"github.com/siyouyun-open/siyouyun_sdk/pkg/utils"
)

type handler func(ugn *utils.UserGroupNamespace, content, uuid string) (reply bool, replyContent string, replyToUUID bool)

type Message struct {
	appCode *string
	nc      *nats.Conn

	mu sync.Mutex
	//triggerPhrasePerls map[string]string
	handlers map[string]handler
}

func NewMessage(appCode *string, nc *nats.Conn) *Message {
	m := &Message{
		appCode: appCode,
		nc:      nc,
		mu:      sync.Mutex{},
		//triggerPhrasePerls: make(map[string]string),
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

// SendMsg sends a message to users. Only users that own this app receive it.
// ugn     : user and namespace
// content : message body
func (m *Message) SendMsg(ugn *utils.UserGroupNamespace, content, replyUUID string) error {
	return gateway.SendMessage(ugn, *m.appCode, content, replyUUID)
}

// AddHandler registers a message robot handler.
// desc:
//
//	description of the message robot
//
// triggerPhrasePerl:
//
//	phrase pattern (regular expression) that triggers the handler
//
// handler func(content string) (reply bool, replyContent string, replyToUUID bool):
//
//	Parameters:
//		- content: message body the user sent to the robot
//	Returns:
//		- reply        : whether a reply is needed
//		- replyContent : body of the reply
//		- replyToUUID  : whether to quote the user message when replying
func (m *Message) AddHandler(desc string, triggerPhrasePerl string, handler func(ugn *utils.UserGroupNamespace, content, uuid string) (reply bool, replyContent string, replyToUUID bool)) {
	if desc == "" {
		// the handler name must not be empty
		return
	}
	//if triggerPhrasePerl == "" {
	// the trigger condition must not be empty
	//return
	//}
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.handlers[desc]; ok {
		return
	} else {
		sdklog.Logger.Infof("registering message robot handler: [%v]", desc)
		m.handlers[desc] = handler
		//m.triggerPhrasePerls[desc] = triggerPhrasePerl
	}
}

// MessageEvent is the structure a message takes when passed through an event.
type MessageEvent struct {
	UGN       utils.UserGroupNamespace `json:"ugn"`
	UUID      string                   `json:"uuid"`
	Content   string                   `json:"content"`
	SessionId string                   `json:"sessionId"`

	SendByAdmin bool `json:"sendByAdmin"`
}

// start the listener
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
