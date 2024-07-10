package utils

import (
	"github.com/kataras/iris/v12"
	"github.com/siyouyun-open/siyouyun_sdk/pkg/const"
	"path/filepath"
	"strings"
)

type UserGroupNamespace struct {
	Username  string `json:"username"`  // 用户名
	GroupName string `json:"groupname"` // 组名
	Namespace string `json:"namespace"` // 命名空间
}

func NewUserNamespaceFromIris(ctx iris.Context) *UserGroupNamespace {
	return &UserGroupNamespace{
		Username:  strings.TrimSpace(ctx.GetHeader(sdkconst.UsernameHeader)),
		GroupName: strings.TrimSpace(ctx.GetHeader(sdkconst.GroupNameHeader)),
		Namespace: strings.TrimSpace(ctx.GetHeader(sdkconst.NamespaceHeader)),
	}
}

func NewUserGroupNamespace(username, groupname, namespace string) *UserGroupNamespace {
	return &UserGroupNamespace{
		Username:  username,
		GroupName: groupname,
		Namespace: namespace,
	}
}

func (ugn *UserGroupNamespace) GetRealPrefix() string {
	if ugn.GroupName == "" {
		ugn.GroupName = ugn.Username
	}
	return filepath.Join(sdkconst.SiyouFSMountPrefix, sdkconst.UserSpacePrefix+ugn.GroupName, ugn.Namespace)
}

func (ugn *UserGroupNamespace) DatabaseName() string {
	if ugn.GroupName == "" {
		ugn.GroupName = ugn.Username
	}
	return sdkconst.SiyouFSMysqlDBPrefix + "_" + ugn.GroupName + "_" + ugn.Namespace
}

func (ugn *UserGroupNamespace) String() string {
	if ugn.GroupName == "" {
		ugn.GroupName = ugn.Username
	}
	return ugn.GroupName + "-" + ugn.Namespace
}
