package utils

import (
	"hash/fnv"
	"path/filepath"
	"strings"

	"github.com/kataras/iris/v12"

	sdkconst "github.com/siyouyun-open/siyouyun_sdk/pkg/const"
)

type UserGroupNamespace struct {
	Username  string `json:"username"`
	GroupName string `json:"groupname"`
	Namespace string `json:"namespace"`
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

func (ugn *UserGroupNamespace) GetRealPrefix(poolName string) string {
	if ugn.GroupName == "" {
		ugn.GroupName = ugn.Username
	}
	if poolName == "" {
		poolName = sdkconst.SiyouSysPool
	}
	return filepath.Join(sdkconst.UserHomeDir, ugn.GroupName, ugn.Namespace, poolName)
}

func (ugn *UserGroupNamespace) SchemaName() string {
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

func (ugn *UserGroupNamespace) HashInt() int64 {
	h := fnv.New64a()
	h.Write([]byte(ugn.String()))
	return int64(h.Sum64() % (1 << 62))
}

// UGNExt ugn extension
type UGNExt struct {
	UserGroupNamespace
	NamespaceAlias string `json:"namespaceAlias"` // namespace alias
	PoolName       string `json:"poolName"`       // group space is only in the specific storage pool
	Quota          uint64 `json:"quota"`          // space quota
}
