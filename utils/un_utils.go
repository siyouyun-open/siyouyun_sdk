package utils

import (
	"github.com/kataras/iris/v12"
	"github.com/siyouyun-open/siyouyun_sdk/const"
	"strings"
)

type UserNamespace struct {
	Username  string `json:"username"`  // 用户名
	GroupName string `json:"groupName"` // 组名
	Namespace string `json:"namespace"` // 命名空间
}

func NewUserNamespaceFromIris(ctx iris.Context) *UserNamespace {
	// username
	username := strings.TrimSpace(ctx.GetHeader(sdkconst.UsernameHeader))
	// namespace
	namespace := strings.TrimSpace(ctx.GetHeader(sdkconst.NamespaceHeader))
	return &UserNamespace{
		Username:  username,
		Namespace: namespace,
	}
}

func NewUserNamespace(username, namespace string) *UserNamespace {
	return &UserNamespace{
		Username:  username,
		Namespace: namespace,
	}
}

func (un *UserNamespace) DatabaseName() string {
	if un.GroupName == "" {
		un.GroupName = un.Username
	}
	return sdkconst.SiyouFSMysqlDBPrefix + "_" + un.GroupName + "_" + un.Namespace
}
