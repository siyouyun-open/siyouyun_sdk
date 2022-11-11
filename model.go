package siyouyunsdk

import (
	"gorm.io/gorm"
)

func (a *AppStruct) WithModel(models ...interface{}) {
	a.Model = append(a.Model, models)
	var ul = a.AppInfo.UserNamespaceList
	for i := range ul {
		a.exec(&ul[i], func(db *gorm.DB) error {
			err := db.AutoMigrate(models...)
			if err != nil {
				return err
			}
			return nil
		})
	}
}
