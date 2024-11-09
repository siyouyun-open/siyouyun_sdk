package siyouyunsdk

import (
	"github.com/siyouyun-open/siyouyun_sdk/pkg/utils"
	"gorm.io/gorm"
)

// ModelMigrator migrate schema according to the model
type ModelMigrator struct {
	models []any
}

func NewModelMigrator(models ...any) *ModelMigrator {
	return &ModelMigrator{
		models: models,
	}
}

func (m *ModelMigrator) Migrate(ugn *utils.UserGroupNamespace) error {
	if len(m.models) == 0 {
		return nil
	}
	fs := App.Ability.FS().NewFSFromUserGroupNamespace(ugn)
	return fs.Exec(func(db *gorm.DB) error {
		return db.AutoMigrate(m.models...)
	})
}
