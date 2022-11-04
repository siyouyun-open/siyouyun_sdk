package utils

import (
	"github.com/kataras/iris/v12"
	"github.com/siyouyun-open/siyouyun_sdk/const"
	"strings"
)

const (
	DatabaseCommon = sdkconst.SiyouFSMysqlDBPrefix + "_" + sdkconst.CommonNamespace
)

type UserNamespace struct {
	Username  string `json:"username"`
	Namespace string `json:"namespace"`
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
	switch un.Namespace {
	case "":
		fallthrough
	case sdkconst.MainNamespace:
		return sdkconst.SiyouFSMysqlDBPrefix + "_" + un.Username + "_" + sdkconst.MainNamespace
	case sdkconst.PrivateNamespace:
		return sdkconst.SiyouFSMysqlDBPrefix + "_" + un.Username + "_" + sdkconst.PrivateNamespace
	case sdkconst.CommonNamespace:
		return sdkconst.SiyouFSMysqlDBPrefix + "_" + sdkconst.CommonNamespace
	}
	return ""
}
