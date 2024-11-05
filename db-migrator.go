package siyouyunsdk

import (
	"github.com/siyouyun-open/siyouyun_sdk/pkg/utils"
	"gorm.io/gorm"
	"hash/fnv"
	"log"
)

// WithModel auto migrate models
// Automatically migrate table models, which is a simplified version of WithSchemaMigrator
func (a *AppStruct) WithModel(models ...any) {
	if len(models) == 0 {
		return
	}
	a.migrateSchema = func(ugn *utils.UserGroupNamespace) error {
		fs := a.Ability.FS().NewFSFromUserGroupNamespace(ugn)
		lockKey := hashStringToInt(ugn.String())
		return fs.Exec(func(tx *gorm.DB) error {
			// Get schema migration lock
			err := tx.Exec("SELECT pg_advisory_xact_lock(?)", lockKey).Error
			if err != nil {
				return err
			}
			return tx.AutoMigrate(models...)
		})
	}
	for i := range a.appInfo.UGNList {
		if err := a.migrateSchema(&a.appInfo.UGNList[i]); err != nil {
			log.Printf("[ERROR] WithModel err: %v", err)
			return
		}
	}
}

// WithSchemaMigrator Migrate schema
// Migrated when invoked and user registered
func (a *AppStruct) WithSchemaMigrator(migrateSchema func(ugn *utils.UserGroupNamespace) error) {
	if migrateSchema == nil {
		return
	}
	a.migrateSchema = migrateSchema
	for i := range a.appInfo.UGNList {
		if err := migrateSchema(&a.appInfo.UGNList[i]); err != nil {
			log.Printf("[ERROR] WithSchemaMigrator err: %v", err)
			return
		}
	}
}

// WithDataMigrator Migrate data
// Migrated when user registered
func (a *AppStruct) WithDataMigrator(migrateData func(ugn *utils.UserGroupNamespace) error) {
	a.migrateData = migrateData
}

// migrateWithUser migrate scheme and data when user register, auto trigger
func (a *AppStruct) migrateWithUser(ugn *utils.UserGroupNamespace) {
	// migrate schema
	if a.migrateSchema != nil {
		if err := a.migrateSchema(ugn); err != nil {
			log.Printf("[ERROR] migrateWithUser migrate schema err: %v", err)
			return
		}
	}
	// migrate data
	if a.migrateData != nil {
		if err := a.migrateData(ugn); err != nil {
			log.Printf("[ERROR] migrateWithUser migrate data err: %v", err)
			return
		}
	}
}

func hashStringToInt(s string) int64 {
	h := fnv.New64a()
	h.Write([]byte(s))
	return int64(h.Sum64() % (1 << 62))
}
