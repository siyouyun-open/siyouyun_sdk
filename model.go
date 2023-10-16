package siyouyunsdk

import (
	"github.com/siyouyun-open/siyouyun_sdk/utils"
	"gorm.io/gorm"
	"log"
)

func (a *AppStruct) WithModel(models ...interface{}) {
	a.Model = append(a.Model, models...)
	var ul = a.AppInfo.UGNList
	for i := range ul {
		_ = a.exec(&ul[i], func(db *gorm.DB) error {
			return db.AutoMigrate(models...)
		})
	}
}

// 增加用户追加建立数据表
func (a *AppStruct) setUserWithModel(un *utils.UserGroupNamespace) {
	_ = a.exec(un, func(db *gorm.DB) error {
		err := db.AutoMigrate(App.Model...)
		if err != nil {
			log.Printf(err.Error())
			return err
		}
		return nil
	})
}
