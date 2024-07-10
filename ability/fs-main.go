package ability

import (
	"github.com/kataras/iris/v12"
	"github.com/siyouyun-open/siyouyun_sdk/internal/gateway"
	"github.com/siyouyun-open/siyouyun_sdk/pkg/utils"
	"gorm.io/gorm"
	"path/filepath"
)

const (
	SiyouyunPrefix = "/.siyouyun"
	AppPrefix      = SiyouyunPrefix + "/appdata"
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

func (f *FS) Close() {
}

func (f *FS) newCommonFS(ugn *utils.UserGroupNamespace) *SyyFS {
	return &SyyFS{
		ugn: ugn,
		db:  f.db,
		api: gateway.NewStorageApi(ugn),
	}
}

func (f *FS) NewFSFromCtx(ctx iris.Context) GenericFS {
	fs := f.newCommonFS(utils.NewUserNamespaceFromIris(ctx))
	fs.root = "/"
	return fs
}

func (f *FS) NewFSFromUserGroupNamespace(ugn *utils.UserGroupNamespace) GenericFS {
	fs := f.newCommonFS(ugn)
	fs.root = "/"
	return fs
}

func (f *FS) NewAppFSFromCtx(ctx iris.Context) GenericFS {
	fs := f.newCommonFS(utils.NewUserNamespaceFromIris(ctx))
	fs.root = filepath.Join(AppPrefix, *f.appCode)
	return fs
}

func (f *FS) NewAppFSFromUserGroupNamespace(ugn *utils.UserGroupNamespace) GenericFS {
	fs := f.newCommonFS(ugn)
	fs.root = filepath.Join(AppPrefix, *f.appCode)
	return fs
}
