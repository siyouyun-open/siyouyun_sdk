package siyouyunsdk

import (
	"github.com/siyouyun-open/siyouyun_sdk/pkg/utils"
	"gorm.io/gorm"
	"log"
)

// WithSchemaMigrator Migrate db schema
// The schema is migrated when the application is started
// After the migrateSchema processing of each UGN is completed, afterFunc is executed (After the transaction is committed)
func (a *AppStruct) WithSchemaMigrator(migrateSchema func(db *gorm.DB) error, afterFunc func(*utils.UserGroupNamespace) error) {
	if migrateSchema == nil {
		return
	}
	a.migrateSchema = migrateSchema
	a.schemaAfterFunc = afterFunc
	for i := range a.appInfo.UGNList {
		ugn := &a.appInfo.UGNList[i]
		fs := a.Ability.FS().NewFSFromUserGroupNamespace(ugn)
		err := fs.Exec(func(db *gorm.DB) error {
			return migrateSchema(db)
		})
		if err != nil {
			log.Printf("[ERROR] WithSchemaMigrator schema err: %v", err)
			return
		}
		// Execute outside the transaction
		err = afterFunc(ugn)
		if err != nil {
			log.Printf("[ERROR] WithSchemaMigrator schema after func err: %v", err)
			return
		}
	}
}

// WithDataMigrator Migrate db data
// The data is migrated when the user is registered
// After the migrateData processing of each UGN is completed, afterFunc is executed (After the transaction is committed)
func (a *AppStruct) WithDataMigrator(migrateData func(db *gorm.DB) error, afterFunc func(*utils.UserGroupNamespace) error) {
	if migrateData == nil {
		return
	}
	a.migrateData = migrateData
	a.dataAfterFunc = afterFunc
}

// migrateWithUser migrate scheme and data when user register, auto trigger
func (a *AppStruct) migrateWithUser(ugn *utils.UserGroupNamespace) {
	fs := a.Ability.FS().NewFSFromUserGroupNamespace(ugn)
	// migrate schema
	if a.migrateSchema != nil {
		err := fs.Exec(func(db *gorm.DB) error {
			return a.migrateSchema(db)
		})
		if err != nil {
			log.Printf("[ERROR] migrateWithUser schema err: %v", err)
			return
		}
	}
	// handle schema after func
	if a.schemaAfterFunc != nil {
		err := a.schemaAfterFunc(ugn)
		if err != nil {
			log.Printf("[ERROR] migrateWithUser schema after func err: %v", err)
			return
		}
	}

	// migrate data
	if a.migrateData != nil {
		err := fs.Exec(func(db *gorm.DB) error {
			return a.migrateData(db)
		})
		if err != nil {
			log.Printf("[ERROR] migrateWithUser data err: %v", err)
			return
		}
	}
	// handle data after func
	if a.dataAfterFunc != nil {
		err := a.dataAfterFunc(ugn)
		if err != nil {
			log.Printf("[ERROR] migrateWithUser data after func err: %v", err)
			return
		}
	}
}
