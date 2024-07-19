package ability

import (
	"encoding/json"
	"github.com/nats-io/nats.go"
	"github.com/siyouyun-open/siyouyun_sdk/internal/gateway"
	"github.com/siyouyun-open/siyouyun_sdk/pkg/utils"
	"log"
	"sync"
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

func (m *Message) Close() {
}

// SendMsg 发送消息给用户,只有权限发送给拥有此app的用户
// ugn		:	用户与空间
// content 	:	消息正文
func (m *Message) SendMsg(ugn *utils.UserGroupNamespace, content, replyUUID string) error {
	return gateway.SendMessage(ugn, *m.appCode, content, replyUUID)
}

// AddHandler 添加消息机器人处理器
// desc:
//
//	消息机器人的功能描述
//
// triggerPhrasePerl:
//
//	触发处理器的短语模式正则
//
// handler func(content string) (reply bool, replyContent string, replyToUUID bool):
//
//	入参:
//		- content 用户发送到机器人的消息正文
//	返回值:
//		- reply 		:	是否需要回复
//		- replyContent	:	回复的正文
//		- replyToUUID	:	回复时是否引用用户消息
func (m *Message) AddHandler(desc string, triggerPhrasePerl string, handler func(ugn *utils.UserGroupNamespace, content, uuid string) (reply bool, replyContent string, replyToUUID bool)) {
	if desc == "" {
		// 处理器命名不能为空
		return
	}
	//if triggerPhrasePerl == "" {
	// 触发条件不能为空
	//return
	//}
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.handlers[desc]; ok {
		return
	} else {
		log.Printf("添加消息机器人处理器:[%v]", desc)
		m.handlers[desc] = handler
		//m.triggerPhrasePerls[desc] = triggerPhrasePerl
	}
}

// MessageEvent 消息在事件中传递的结构
type MessageEvent struct {
	UGN       utils.UserGroupNamespace `json:"ugn"`
	UUID      string                   `json:"uuid"`
	Content   string                   `json:"content"`
	SessionId string                   `json:"sessionId"`

	SendByAdmin bool `json:"sendByAdmin"`
}

// 开启监听器
func (m *Message) enableListener() {
	robotCode := *m.appCode + "_msg"
	go func() {
		log.Printf("start ListenBizMsg at:%v", robotCode)
		_, _ = m.nc.Subscribe(robotCode, func(msg *nats.Msg) {
			var mes []MessageEvent
			defer func() {
				if err := recover(); err != nil {
					log.Printf("nats panic:[%v]-[%v]", err, mes)
				}
			}()
			err := json.Unmarshal(msg.Data, &mes)
			if err != nil {
				return
			}
			for i := range mes {
				ugn := utils.NewUserGroupNamespace(mes[i].UGN.Username, mes[i].UGN.GroupName, mes[i].UGN.Namespace)
				if !mes[i].SendByAdmin {
					//var handlers []handler
					//for i1 := range m.triggerPhrasePerls {
					// 解析正文匹配那个正则处理器
					//match, err := regexp.Match(m.triggerPhrasePerls[i1], []byte(mes[i].Content))
					//if err != nil {
					//	continue
					//}
					//if match {
					//	if _, ok := m.handlers[i1]; ok {
					//handlers = append(handlers, m.handlers[i1])
					//}
					//}
					//}
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
	}()
}
