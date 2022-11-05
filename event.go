package siyouyunfaas

import (
	"crypto/md5"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/nats-io/nats.go"
	"github.com/siyouyun-open/siyouyun_sdk/const"
	"github.com/siyouyun-open/siyouyun_sdk/entity"
	"github.com/siyouyun-open/siyouyun_sdk/utils"
	"gorm.io/gorm"
	"strconv"
)

//goland:noinspection GoCommentStart
const (
	// mimeType main
	FileTypeAll      FileType = "all"
	FileTypeText     FileType = "text"
	FileTypeImage    FileType = "image"
	FileTypeAudio    FileType = "audio"
	FileTypeVideo    FileType = "video"
	FileTypeMessage  FileType = "message"
	FileTypeCompress FileType = "compress"

	// other type
	FileTypeImageVideo FileType = "image-video"
	FileTypeDoc        FileType = "doc"
	FileTypeBt         FileType = "bt"
	FileTypeEbook      FileType = "ebook"
	FileTypeSoftware   FileType = "software"
	FileTypeOther      FileType = "other"
)

const (
	FileEventAdd = iota + 1
	FileEventDelete
)

const (
	EventStatusError  = 3
	EventStatusFinish = 4
)

type FileType string

type FileEvent struct {
	Inode     int64  `json:"inode,omitempty"`
	Action    int    `json:"action,omitempty"`
	Username  string `json:"username,omitempty"`
	Namespace string `json:"namespace,omitempty"`
}

type EventHolder struct {
	app     *App
	name    string
	options []PreferOptions
}

type PreferOptions struct {
	FileType      FileType
	FileEventType int
	Description   string
	Handler       func(fs *FS) error
}

// NewEventHolder 初始化事件监听器
func NewEventHolder(appName string) *EventHolder {
	return &EventHolder{
		name: appName,
	}
}

// SetPrefer 设置偏好与回调函数
func (e *EventHolder) SetPrefer(options ...PreferOptions) {
	e.options = append(e.options, options...)
}

// Listen 开始监听器工作
func (e *EventHolder) Listen() {
	go func() {
		e.cleanAppRegisterInfo()
		for i := range e.options {
			var ari = &entity.ActionAppRegisterInfo{
				AppCodeName: e.name,
				EventType:   e.options[i].FileEventType,
				FileType:    string(e.options[i].FileType),
				Description: e.options[i].Description,
			}
			ari.Code = getAppRegisterInfoCode(ari)
			// 处理用户的event注册信息
			e.app.execByEvent(utils.NewUserNamespace("", sdkconst.CommonNamespace), func(db *gorm.DB) error {
				return e.registerIfHaveApp(db, ari)
			})
			var ul = e.app.AppInfo.RegisterUserList
			for i := range ul {
				e.app.execByEvent(utils.NewUserNamespace(ul[i], sdkconst.MainNamespace), func(db *gorm.DB) error {
					return e.registerIfHaveApp(db, ari)
				})
				e.app.execByEvent(utils.NewUserNamespace(ul[i], sdkconst.PrivateNamespace), func(db *gorm.DB) error {
					return e.registerIfHaveApp(db, ari)
				})
			}
			//启动监听event
			nc := getNats()
			j := i
			_, _ = nc.Subscribe(ari.Code, func(msg *nats.Msg) {
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
				fs := newEventFSFromFileEvent(ari.AppCodeName, &fe)
				err = e.options[j].Handler(fs)
				if err != nil {
					_ = nc.Publish(msg.Reply, []byte(strconv.Itoa(EventStatusError)))
				}
				_ = nc.Publish(msg.Reply, []byte(strconv.Itoa(EventStatusFinish)))
			})
		}
	}()
}

// 清理app事件注册信息
func (e *EventHolder) cleanAppRegisterInfo() {
	doClean := func(db *gorm.DB, appName string) error {
		return db.Where("app_code_name = ?", appName).Delete(&entity.ActionAppRegisterInfo{}).Error
	}
	e.app.execByEvent(utils.NewUserNamespace("", sdkconst.CommonNamespace), func(db *gorm.DB) error {
		err := doClean(db, e.name)
		if err != nil {
			return err
		}
		return nil
	})
	var ul = e.app.AppInfo.RegisterUserList
	for i := range ul {
		e.app.execByEvent(utils.NewUserNamespace(ul[i], sdkconst.MainNamespace), func(db *gorm.DB) error {
			err := doClean(db, e.name)
			if err != nil {
				return err
			}
			return nil
		})
		e.app.execByEvent(utils.NewUserNamespace(ul[i], sdkconst.PrivateNamespace), func(db *gorm.DB) error {
			err := doClean(db, e.name)
			if err != nil {
				return err
			}
			return nil
		})
	}
}

// 当有app时增加注册信息
func (e *EventHolder) registerIfHaveApp(db *gorm.DB, ari *entity.ActionAppRegisterInfo) error {
	var app entity.Apps
	err := db.Where(entity.Apps{CodeName: ari.AppCodeName}).First(&app).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		// 尝试清理register info
		err = db.Delete(&entity.ActionAppRegisterInfo{}, "app_code_name = ?", ari.AppCodeName).Error
		return err
	}
	// 增加register info
	var old entity.ActionAppRegisterInfo
	err = db.Where(entity.ActionAppRegisterInfo{
		AppCodeName: ari.AppCodeName,
		EventType:   ari.EventType,
		FileType:    ari.FileType,
		Description: ari.Description,
	}).Delete(&old).Error
	if err != nil {
		return err
	}
	err = db.Create(ari).Error
	if err != nil {
		return err
	}
	return nil
}

// 拼接app事件code
func getAppRegisterInfoCode(ari *entity.ActionAppRegisterInfo) string {
	return fmt.Sprintf("%x", md5.Sum([]byte(fmt.Sprintf("%v%v%v%v", ari.AppCodeName, ari.EventType, ari.FileType, ari.Description))))
}

func getNats() *nats.Conn {
	nc, err := nats.Connect("nats://10.62.0.1:4222")
	if err != nil {
		return nil
	}
	return nc
}