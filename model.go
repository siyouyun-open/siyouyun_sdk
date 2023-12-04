package siyouyunsdk

import (
	"github.com/siyouyun-open/siyouyun_sdk/pkg/utils"
	"gorm.io/gorm"
	"log"
)

// WithModel 自动迁移表
func (a *AppStruct) WithModel(models ...interface{}) {
	a.Model = append(a.Model, models...)
	var ul = a.AppInfo.UGNList
	for i := range ul {
		_ = a.exec(&ul[i], func(db *gorm.DB) error {
			return db.AutoMigrate(models...)
		})
	}
}

// UpdateModel 更新表（需要删除更改字段或索引时使用）
func (a *AppStruct) UpdateModel(f func(gorm.Migrator)) {
	var ul = a.AppInfo.UGNList
	for i := range ul {
		_ = a.exec(&ul[i], func(db *gorm.DB) error {
			f(db.Migrator())
			return nil
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
