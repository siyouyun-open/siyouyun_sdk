package siyouyunsdk

import (
	"crypto/md5"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/nats-io/nats.go"
	"github.com/siyouyun-open/siyouyun_sdk/pkg/const"
	"github.com/siyouyun-open/siyouyun_sdk/pkg/restclient"
	"github.com/siyouyun-open/siyouyun_sdk/pkg/utils"
	"golang.org/x/exp/maps"
	"log"
	"strconv"
)

// MediaType 文件媒体类型
type MediaType string

// 偏好设置可以关注的文件媒体类型，上半部分为标准媒体类型，下半部分为自定义媒体类型
// 标准媒体类型
const (
	MediaTypeText    MediaType = "text"
	MediaTypeImage   MediaType = "image"
	MediaTypeAudio   MediaType = "audio"
	MediaTypeVideo   MediaType = "video"
	MediaTypeMessage MediaType = "message"
)

// 自定义媒体类型
const (
	MediaTypeAll        MediaType = "all"         // 全部类型
	MediaTypeCompress   MediaType = "compress"    // 压缩包类型
	MediaTypeImageVideo MediaType = "image-video" // 图片+视频类型
	MediaTypeDoc        MediaType = "doc"         // 文档类型
	MediaTypeBt         MediaType = "bt"          // 种子类型
	MediaTypeEbook      MediaType = "ebook"       // 电子书类型
	MediaTypeSoftware   MediaType = "software"    // 软件包类型
	MediaTypeOther      MediaType = "other"       // 其他类型
)

// 文件事件类型
const (
	FileEventAdd    = iota + 1 // 文件创建
	FileEventDelete            // 文件删除
	FileEventRename            // 文件重命名
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
	UGN        *utils.UserGroupNamespace `json:"ugn"`
	OldUFI     string                    `json:"oldUfi"`
	UFI        string                    `json:"ufi"`
	Inode      uint64                    `json:"inode"`
	Action     int                       `json:"action"`
	WithAvatar bool                      `json:"withAvatar"`
}

type EventHolder struct {
	app        *AppStruct
	optionsMap map[string]PreferOptions
}

// PreferOptions 事件偏好选项
type PreferOptions struct {
	MediaType     MediaType                                   `json:"mediaType"`     // 媒体类型
	FileEventType int                                         `json:"fileEventType"` // 事件类型
	FollowDirs    []string                                    `json:"followDirs"`    // 关注目录（不设置默认所有）
	ExcludeExts   []string                                    `json:"excludeExts"`   // 排除的文件后缀
	Description   string                                      `json:"description"`   // 描述
	Priority      TaskLevel                                   `json:"priority"`      // 优先级（资源占用等级）
	Handler       func(fe *FileEvent) (ConsumeStatus, string) `json:"-"`             // 处理器
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
}

// Listen 开始监听器工作
func (e *EventHolder) Listen() {
	if len(e.optionsMap) == 0 {
		return
	}
	var err error
	//启动监听event
	nc := e.app.nc
	if nc == nil {
		return
	}
	err = registerAppEvent(e.app.AppCode, maps.Values(e.optionsMap))
	if err != nil {
		panic(err)
	}
	go func() {
		_, err = nc.Subscribe(e.app.AppCode+"_event", func(msg *nats.Msg) {
			defer func() {
				if err := recover(); err != nil {
					log.Printf("[PANIC] event handler panic: %v", err)
					return
				}
			}()
			var fe FileEvent
			err := json.Unmarshal(msg.Data, &fe)
			if err != nil {
				return
			}
			eventCode := msg.Header.Get("eventCode")

			// 异步执行具体任务
			go func() {
				cs, m := e.optionsMap[eventCode].Handler(&fe)
				var resMsg = nats.NewMsg(msg.Reply)
				resMsg.Data = []byte(m)
				resMsg.Header.Set("status", strconv.Itoa(int(cs)))
				_ = nc.PublishMsg(resMsg)
			}()
		})
		if err != nil {
			log.Printf("[ERROR] EventHolder subscribe err: %v", err)
		}
	}()
}

// 拼接app事件code
func (p *PreferOptions) parseToEventCode(appCode string) string {
	return fmt.Sprintf("%x", md5.Sum([]byte(fmt.Sprintf("%v%v%v%v", appCode, p.FileEventType, p.MediaType, p.Description))))
}

func registerAppEvent(appCode string, options []PreferOptions) error {
	api := utils.GetOSServiceURL() + "/faas/app/event/register"
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
