package ability

import (
	"path/filepath"

	"github.com/kataras/iris/v12"
	"github.com/siyouyun-open/siyouyun_sdk/pkg/utils"
	"gorm.io/gorm"
)

const (
	AppPrefix = "/.siyouyun/appdata"
)

type FS struct {
	appCode *string
	db      *gorm.DB
}

func NewFS(appCode *string, db *gorm.DB) *FS {
	return &FS{
		appCode: appCode,
		db:      db,
	}
}

func (f *FS) Name() string {
	return "FS"
}

func (f *FS) IsReady() bool {
	return true
}

func (f *FS) Close() {
}

func (f *FS) NewFSFromCtx(ctx iris.Context) GenericFS {
	return f.NewFSFromUserGroupNamespace(utils.NewUserNamespaceFromIris(ctx))
}

func (f *FS) NewFSFromUserGroupNamespace(ugn *utils.UserGroupNamespace) GenericFS {
	return &SyyFS{
		ugn:       ugn,
		appPrefix: filepath.Join(AppPrefix, *f.appCode),
		db:        f.db,
	}
}
