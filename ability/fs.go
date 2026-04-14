package ability

import (
	"path/filepath"

	"github.com/kataras/iris/v12"
	"gorm.io/gorm"

	sdkconst "github.com/siyouyun-open/siyouyun_sdk/pkg/const"
	sdkdto "github.com/siyouyun-open/siyouyun_sdk/pkg/dto"
	"github.com/siyouyun-open/siyouyun_sdk/pkg/restclient"
	"github.com/siyouyun-open/siyouyun_sdk/pkg/utils"
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

// GetUGNList gets ugn list
func (f *FS) GetUGNList(ctx iris.Context) []utils.UGNExt {
	api := utils.GetOSServiceURL() + "/fs/group/space/list"
	currentUGN := utils.NewUserNamespaceFromIris(ctx)
	response := restclient.GetRequest[[]sdkdto.GroupStorageSpaceInfo](currentUGN, api, nil)
	list := []utils.UGNExt{
		{
			UserGroupNamespace: utils.UserGroupNamespace{
				Username:  currentUGN.Username,
				GroupName: currentUGN.Username,
				Namespace: sdkconst.MainNamespace,
			},
		},
		{
			UserGroupNamespace: utils.UserGroupNamespace{
				Username:  currentUGN.Username,
				GroupName: currentUGN.Username,
				Namespace: sdkconst.PrivateNamespace,
			},
		},
	}
	if response.Code == sdkconst.Success && response.Data != nil {
		for _, item := range *response.Data {
			list = append(list, utils.UGNExt{
				UserGroupNamespace: utils.UserGroupNamespace{
					Username:  currentUGN.Username,
					GroupName: sdkconst.CommonNamespace,
					Namespace: item.Namespace,
				},
				NamespaceAlias: item.NamespaceAlias,
				PoolName:       item.PoolName,
				Quota:          item.Quota,
			})
		}
	}
	return list
}
