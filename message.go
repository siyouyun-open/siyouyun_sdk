package siyouyunsdk

import (
	"encoding/json"
	"github.com/nats-io/nats.go"
	"github.com/siyouyun-open/siyouyun_sdk/gateway"
	"github.com/siyouyun-open/siyouyun_sdk/utils"
	"log"
)

type Message struct {
}

// SendMsg 发送消息给用户,只有权限发送给拥有此app的用户
// un		:	用户与空间
// content 	:	消息正文
func (m *Message) SendMsg(un *utils.UserGroupNamespace, content string) error {
	return gateway.SendMessage(un, App.AppCode, content, "")
}

// MessageEvent 消息在事件中传递的结构
type MessageEvent struct {
	Username  string `json:"username"`
	Groupname string `json:"groupname"`
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

func ListenMsg(mh *MessageHandlerStruct) {
	nc := getNats()
	go func() {
		log.Printf("start ListenMsg at:%v", mh.RobotCode)
		_, _ = nc.Subscribe(mh.RobotCode, func(msg *nats.Msg) {
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
				un := utils.NewUserGroupNamespace(mes[i].Username, mes[i].Groupname, mes[i].Namespace)
				if mes[i].SendByAdmin {
					switch mes[i].Content {
					case "autoMigrate":
						log.Printf("mes[i].Content:%v", mes[i].Content)
						App.setUserWithModel(un)
					}
				} else {
					fs := App.NewAppFSFromUserNamespace(un)
					// 获取消息正文
					reply, content, replyToUUID := mh.Handler(fs, mes[i].Content)
					if reply {
						var replyUUID string
						if replyToUUID {
							replyUUID = mes[i].UUID
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
