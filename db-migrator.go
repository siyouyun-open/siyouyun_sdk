package siyouyunsdk

import (
	"github.com/siyouyun-open/siyouyun_sdk/pkg/utils"
	"gorm.io/gorm"
	"log"
)

type MigrateFunc func(db *gorm.DB, ugn *utils.UserGroupNamespace) error

// WithMigrator Migrate db, including schema and data.
// The schema is migrated when the application is started,
// and the data is migrated when the user is registered
func (a *AppStruct) WithMigrator(migrateSchema MigrateFunc, migrateData MigrateFunc) {
	if migrateSchema == nil {
		return
	}
	a.migrateSchema = migrateSchema
	a.migrateData = migrateData
	for i := range a.appInfo.UGNList {
		ugn := &a.appInfo.UGNList[i]
		fs := a.Ability.FS().NewFSFromUserGroupNamespace(ugn)
		err := fs.Exec(func(db *gorm.DB) error {
			return migrateSchema(db, ugn)
		})
		if err != nil {
			log.Printf("[ERROR] WithMigrator err: %v", err)
			return
		}
	}
}

// migrateWithUser migrate scheme and data when user register, auto trigger
func (a *AppStruct) migrateWithUser(ugn *utils.UserGroupNamespace) {
	if a.migrateSchema == nil {
		return
	}
	fs := a.Ability.FS().NewFSFromUserGroupNamespace(ugn)
	_ = fs.Exec(func(db *gorm.DB) error {
		err := a.migrateSchema(db, ugn)
		if err != nil {
			log.Printf("[ERROR] migrateWithUser schema err: %v", err)
		}
		if a.migrateData != nil {
			err = a.migrateData(db, ugn)
			if err != nil {
				log.Printf("[ERROR] migrateWithUser data err: %v", err)
			}
		}
		return nil
	})
}
