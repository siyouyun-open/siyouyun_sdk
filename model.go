package siyouyunsdk

import (
	"github.com/siyouyun-open/siyouyun_sdk/const"
	"github.com/siyouyun-open/siyouyun_sdk/utils"
	"gorm.io/gorm"
)

func (a *AppStruct) WithModel(models ...interface{}) {
	a.Model = append(a.Model, models)
	a.exec(utils.NewUserNamespace("", sdkconst.CommonNamespace), func(db *gorm.DB) error {
		err := db.AutoMigrate(models...)
		if err != nil {
			return err
		}
		return nil
	})
	var ul = a.AppInfo.RegisterUserList
	for i := range ul {
		a.exec(utils.NewUserNamespace(ul[i], sdkconst.MainNamespace), func(db *gorm.DB) error {
			err := db.AutoMigrate(models...)
			if err != nil {
				return err
			}
			return nil
		})
		a.exec(utils.NewUserNamespace(ul[i], sdkconst.PrivateNamespace), func(db *gorm.DB) error {
			err := db.AutoMigrate(models...)
			if err != nil {
				return err
			}
			return nil
		})
	}
}
