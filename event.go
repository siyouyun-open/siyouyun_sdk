package siyouyunsdk

import (
	"crypto/md5"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/nats-io/nats.go"
	sdkconst "github.com/siyouyun-open/siyouyun_sdk/const"
	"github.com/siyouyun-open/siyouyun_sdk/gateway"
	"github.com/siyouyun-open/siyouyun_sdk/restclient"
	"strconv"
)

type FileType string

// 偏好设置可以关注的文件类型，上半部分为独立类型文件，下半部分为混合类型文件
const (
	FileTypeAll      FileType = "all"
	FileTypeText     FileType = "text"
	FileTypeImage    FileType = "image"
	FileTypeAudio    FileType = "audio"
	FileTypeVideo    FileType = "video"
	FileTypeMessage  FileType = "message"
	FileTypeCompress FileType = "compress"

	FileTypeImageVideo FileType = "image-video"
	FileTypeDoc        FileType = "doc"
	FileTypeBt         FileType = "bt"
	FileTypeEbook      FileType = "ebook"
	FileTypeSoftware   FileType = "software"
	FileTypeOther      FileType = "other"
)

// 文件事件类型，文件创建与文件删除
const (
	FileEventAdd = iota + 1
	FileEventDelete
)

type ConsumeStatus int

const (
	ConsumeSuccess ConsumeStatus = iota + 1
	ConsumeRetry
	ConsumeFail
)

type TaskLevel int

const (
	HighLevel TaskLevel = iota + 1
	MidLevel
	LowLevel
)

type FileEvent struct {
	Inode     int64  `json:"inode,omitempty"`
	Action    int    `json:"action,omitempty"`
	Username  string `json:"username,omitempty"`
	Namespace string `json:"namespace,omitempty"`
}

type EventHolder struct {
	app        *AppStruct
	options    []PreferOptions
	optionsMap map[string]PreferOptions
}

type PreferOptions struct {
	FileType      FileType                                  `json:"fileType"`
	FileEventType int                                       `json:"fileEventType"`
	Description   string                                    `json:"description"`
	Priority      TaskLevel                                 `json:"priority"`
	Handler       func(fs *EventFS) (ConsumeStatus, string) `json:"-"`
}

// WithEventHolder 初始化事件监听器
func (a *AppStruct) WithEventHolder() {
	a.Event = &EventHolder{
		app:        a,
		optionsMap: make(map[string]PreferOptions),
	}
}

// SetPrefer 设置偏好与回调函数
func (e *EventHolder) SetPrefer(options ...PreferOptions) {
	for i := range options {
		if options[i].Priority == 0 {
			options[i].Priority = LowLevel
		}
		e.optionsMap[options[i].parseToEventCode(e.app.AppCode)] = options[i]
	}
	e.options = append(e.options, options...)
}

// Listen 开始监听器工作
func (e *EventHolder) Listen() {
	if len(e.options) == 0 {
		return
	}
	var err error
	//启动监听event
	nc := getNats()
	if nc == nil {
		return
	}
	err = registerAndGetAppEvent(e.app.AppCode, e.options)
	if err != nil {
		return
	}
	go func() {
		_, _ = nc.Subscribe(e.app.AppCode+"_event", func(msg *nats.Msg) {
			var fe FileEvent
			defer func() {
				if err := recover(); err != nil {
					return
				}
			}()
			err := json.Unmarshal(msg.Data, &fe)
			if err != nil {
				return
			}
			eventCode := msg.Header.Get("eventCode")
			eventfs := e.app.newEventFSFromFileEvent(&fe)
			cs, m := e.optionsMap[eventCode].Handler(eventfs)
			eventfs.Destroy()
			var resMsg = nats.NewMsg(msg.Reply)
			resMsg.Data = []byte(m)
			resMsg.Header.Set("status", strconv.Itoa(int(cs)))
			_ = nc.PublishMsg(resMsg)
			return
		})
	}()
}

// 拼接app事件code
func (p *PreferOptions) parseToEventCode(appCode string) string {
	return fmt.Sprintf("%x", md5.Sum([]byte(fmt.Sprintf("%v%v%v%v", appCode, p.FileEventType, p.FileType, p.Description))))
}

func getNats() *nats.Conn {
	nc, err := nats.Connect("nats://10.62.0.1:4222")
	if err != nil {
		return nil
	}
	return nc
}

func registerAndGetAppEvent(appCode string, options []PreferOptions) error {
	api := gateway.CoreServiceURL + "/faas/app/event/register"
	response := restclient.PostRequest[any](
		nil,
		api,
		map[string]string{"appCode": appCode},
		options,
	)
	if response.Code != sdkconst.Success {
		return errors.New(response.Msg)
	}
	return nil
}
