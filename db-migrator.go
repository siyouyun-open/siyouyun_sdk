package siyouyunsdk

import (
	"github.com/siyouyun-open/siyouyun_sdk/pkg/utils"
	"gorm.io/gorm"
	"log"
)

type MigrateFunc func(db *gorm.DB) error

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
		fs := a.Ability.FS().NewFSFromUserGroupNamespace(&a.appInfo.UGNList[i])
		err := fs.Exec(func(db *gorm.DB) error {
			return migrateSchema(db)
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
		err := a.migrateSchema(db)
		if err != nil {
			log.Printf("[ERROR] migrateWithUser schema err: %v", err)
		}
		if a.migrateData != nil {
			err = a.migrateData(db)
			if err != nil {
				log.Printf("[ERROR] migrateWithUser data err: %v", err)
			}
		}
		return nil
	})
}
