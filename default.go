package siyouyunsdk

import (
	"fmt"
	"os"

	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"

	sdklog "github.com/siyouyun-open/siyouyun_sdk/pkg/log"
	"github.com/siyouyun-open/siyouyun_sdk/pkg/utils"
)

// ModelMigrator migrate schema according to the model
type ModelMigrator struct {
	initSchema        func(*gorm.DB) error
	migrationVersions func() []*gormigrate.Migration
}

func NewModelMigrator(initSchema func(*gorm.DB) error, migrationVersions func() []*gormigrate.Migration) *ModelMigrator {
	return &ModelMigrator{
		initSchema:        initSchema,
		migrationVersions: migrationVersions,
	}
}

func (m *ModelMigrator) Migrate(ugn *utils.UserGroupNamespace) error {
	if m.initSchema == nil && m.migrationVersions == nil {
		return nil
	}
	var migrations []*gormigrate.Migration
	if m.migrationVersions != nil {
		migrations = m.migrationVersions()
	}
	fs := App.Ability.FS().NewFSFromUserGroupNamespace(ugn)
	return fs.Exec(func(db *gorm.DB) error {
		g := gormigrate.New(db, &gormigrate.Options{
			TableName:                 fmt.Sprintf("migrations_%s", os.Getenv(AppCodeEnvKey)),
			IDColumnName:              "id",
			IDColumnSize:              255,
			UseTransaction:            false,
			ValidateUnknownMigrations: false,
		}, migrations)
		g.InitSchema(m.initSchema)
		if err := g.Migrate(); err != nil {
			sdklog.Logger.Errorf("Migrate [%v] err: %v", ugn.String(), err)
		}
		return nil
	})
}
