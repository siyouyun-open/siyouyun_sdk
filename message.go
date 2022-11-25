package siyouyunsdk

import (
	"encoding/json"
	"github.com/nats-io/nats.go"
	"github.com/siyouyun-open/siyouyun_sdk/gateway"
	"github.com/siyouyun-open/siyouyun_sdk/utils"
	"log"
)

type MessageEvents struct {
	Msgs []MessageEvent `json:"msgs"`
}

// MessageEvent 消息在事件中传递的结构
type MessageEvent struct {
	Username  string `json:"username"`
	Namespace string `json:"namespace"`
	UUID      string `json:"uuid"`
	Content   string `json:"content"`
	SessionId string `json:"sessionId"`

	SendByAdmin bool `json:"sendByAdmin"`
}

type MessageHandlerStruct struct {
	RobotCode string                                                                                 `json:"robotCode"`
	RobotDesc string                                                                                 `json:"robotDesc"`
	Handler   func(appfs *AppFS, content string) (reply bool, replyContent string, replyToUUID bool) `json:"-"`
}

var MessageHandler *MessageHandlerStruct

// EnableMessage 开启消息机器人
// desc:
// 	消息机器人的功能描述
// handler func(content string) (reply bool, replyContent string, replyToUUID bool):
// 	入参:
//		- content 用户发送到机器人的消息正文
// 	返回值:
// 		- reply 		:	是否需要回复
// 		- replyContent	:	回复的正文
// 		- replyToUUID	:	回复时是否引用用户消息
func EnableMessage(desc string, handler func(appfs *AppFS, content string) (reply bool, replyContent string, replyToUUID bool)) error {
	// 注册机器人
	err := gateway.RegisterMessageRobot(App.AppCode, desc)
	if err != nil {
		return err
	}
	// 开启监听
	ListenMsg(&MessageHandlerStruct{
		RobotCode: App.AppCode + "_msg", // todo use uuid
		RobotDesc: desc,
		Handler:   handler,
	})
	return nil
}

// SendMsg 发送消息给用户,只有权限发送给拥有此app的用户
// un		:	用户与空间
// content 	:	消息正文
func (mh *MessageHandlerStruct) SendMsg(un *utils.UserNamespace, content string) error {
	return gateway.SendMessage(un, App.AppCode, content, "")
}

func ListenMsg(mh *MessageHandlerStruct) {
	if mh.Handler == nil {
		return
	}
	nc := getNats()
	go func() {
		_, _ = nc.Subscribe(mh.RobotCode, func(msg *nats.Msg) {
			var mes MessageEvents
			defer func() {
				if err := recover(); err != nil {
					log.Printf("nats panic:[%v]-[%v]", err, mes)
				}
			}()
			err := json.Unmarshal(msg.Data, &mes)
			if err != nil {
				return
			}
			for i := range mes.Msgs {
				me := mes.Msgs[i]
				un := utils.NewUserNamespace(me.Username, me.Namespace)
				if me.SendByAdmin {
					switch me.Content {
					case "autoMigrate":
						App.setUserWithModel(un)
					}
				} else {
					fs := App.NewAppFSFromUserNamespace(un)
					// 获取消息正文
					reply, content, replyToUUID := mh.Handler(fs, me.Content)
					if reply {
						var replyUUID string
						if replyToUUID {
							replyUUID = me.UUID
						}
						err = gateway.SendMessage(un, App.AppCode, content, replyUUID)
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
