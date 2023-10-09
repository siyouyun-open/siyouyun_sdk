package utils

import (
	"github.com/kataras/iris/v12"
	"github.com/siyouyun-open/siyouyun_sdk/const"
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

func (un *UserGroupNamespace) DatabaseName() string {
	if un.GroupName == "" {
		un.GroupName = un.Username
	}
	return sdkconst.SiyouFSMysqlDBPrefix + "_" + un.GroupName + "_" + un.Namespace
}

func (un *UserGroupNamespace) String() string {
	if un.GroupName == "" {
		un.GroupName = un.Username
	}
	return un.GroupName + "-" + un.Namespace
}
