package ability

import (
	"encoding/json"
	"github.com/nats-io/nats.go"
	"github.com/siyouyun-open/siyouyun_sdk"
	"github.com/siyouyun-open/siyouyun_sdk/internal/gateway"
	"github.com/siyouyun-open/siyouyun_sdk/pkg/utils"
	"log"
)

type Message struct {
	nc *nats.Conn
}

func NewMessage(nc *nats.Conn) *Message {
	return &Message{
		nc: nc,
	}
}

func (m *Message) Name() string {
	return "Message"
}

func (m *Message) Close() {
}

// SendMsg 发送消息给用户,只有权限发送给拥有此app的用户
// ugn		:	用户与空间
// content 	:	消息正文
func (m *Message) SendMsg(ugn *utils.UserGroupNamespace, content string) error {
	return gateway.SendMessage(ugn, siyouyunsdk.App.AppCode, content, "")
}

// MessageEvent 消息在事件中传递的结构
type MessageEvent struct {
	UGN       utils.UserGroupNamespace `json:"ugn"`
	UUID      string                   `json:"uuid"`
	Content   string                   `json:"content"`
	SessionId string                   `json:"sessionId"`

	SendByAdmin bool `json:"sendByAdmin"`
}

type MessageHandlerStruct struct {
	RobotCode string                                                                                             `json:"robotCode"`
	RobotDesc string                                                                                             `json:"robotDesc"`
	Handler   func(appfs *siyouyunsdk.AppFS, content string) (reply bool, replyContent string, replyToUUID bool) `json:"-"`
}

// EnableMessage 开启消息机器人
// desc:
//
//	消息机器人的功能描述
//
// handler func(content string) (reply bool, replyContent string, replyToUUID bool):
//
//	入参:
//		- content 用户发送到机器人的消息正文
//	返回值:
//		- reply 		:	是否需要回复
//		- replyContent	:	回复的正文
//		- replyToUUID	:	回复时是否引用用户消息
func (m *Message) EnableMessage(desc string, handler func(appfs *siyouyunsdk.AppFS, content string) (reply bool, replyContent string, replyToUUID bool)) error {
	// 开启监听
	m.ListenMsg(&MessageHandlerStruct{
		RobotCode: siyouyunsdk.App.AppCode + "_msg",
		RobotDesc: desc,
		Handler:   handler,
	})
	return nil
}

func (m *Message) ListenMsg(mh *MessageHandlerStruct) {
	go func() {
		log.Printf("start ListenBizMsg at:%v", mh.RobotCode)
		_, _ = m.nc.Subscribe(mh.RobotCode, func(msg *nats.Msg) {
			var mes []MessageEvent
			defer func() {
				if err := recover(); err != nil {
					log.Printf("nats panic:[%v]-[%v]", err, mes)
				}
			}()
			err := json.Unmarshal(msg.Data, &mes)
			if err != nil {
				panic(err)
			}
			for i := range mes {
				ugn := utils.NewUserGroupNamespace(mes[i].UGN.Username, mes[i].UGN.GroupName, mes[i].UGN.Namespace)
				if !mes[i].SendByAdmin {
					fs := siyouyunsdk.App.NewAppFSFromUserGroupNamespace(ugn)
					// 获取消息正文
					reply, content, replyToUUID := mh.Handler(fs, mes[i].Content)
					if reply {
						var replyUUID string
						if replyToUUID {
							replyUUID = mes[i].UUID
						}
						err = gateway.SendMessage(ugn, siyouyunsdk.App.AppCode, content, replyUUID)
						if err != nil {
							return
						}
					}
				}
			}
			return
		})
	}()
}
