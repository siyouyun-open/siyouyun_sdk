package siyouyunsdk

import (
	"github.com/siyouyun-open/siyouyun_sdk/pkg/utils"
	"gorm.io/gorm"
	"log"
)

// WithModel auto migrate tables
func (a *AppStruct) WithModel(models ...interface{}) {
	a.models = append(a.models, models...)
	for i := range a.appInfo.UGNList {
		fs := a.Ability.FS().NewFSFromUserGroupNamespace(&a.appInfo.UGNList[i])
		_ = fs.Exec(func(db *gorm.DB) error {
			err := db.AutoMigrate(models...)
			if err != nil {
				log.Printf(err.Error())
			}
			return err
		})
	}
}

// UpdateModel Update table, used when changing fields or indexes
func (a *AppStruct) UpdateModel(f func(db *gorm.DB)) error {
	for i := range a.appInfo.UGNList {
		fs := a.Ability.FS().NewFSFromUserGroupNamespace(&a.appInfo.UGNList[i])
		err := fs.Exec(func(db *gorm.DB) error {
			f(db)
			return nil
		})
		if err != nil {
			return err
		}
	}
	return nil
}

func (a *AppStruct) setUserWithModel(ugn *utils.UserGroupNamespace) {
	fs := a.Ability.FS().NewFSFromUserGroupNamespace(ugn)
	_ = fs.Exec(func(db *gorm.DB) error {
		err := db.AutoMigrate(a.models...)
		if err != nil {
			log.Printf(err.Error())
		}
		return err
	})
}
