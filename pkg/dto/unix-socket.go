package sdkdto

import (
	"encoding/json"
	"github.com/siyouyun-open/siyouyun_sdk/pkg/utils"
	"os"
)

// UnixFileOperator unix套接字文件操作符
type UnixFileOperator string

const (
	UnixOpen UnixFileOperator = "open" // 打开文件
)

// UnixFileOperateReq unix套接字的文件操作请求
type UnixFileOperateReq struct {
	UGN      *utils.UserGroupNamespace `json:"ugn"`      // 用户组空间
	Operator UnixFileOperator          `json:"operator"` // 文件操作符
	Param    json.RawMessage           `json:"param"`    // 参数详情
}

// UnixOpenFileParam unix打开文件参数
type UnixOpenFileParam struct {
	Name       string      `json:"name"`
	Flag       int         `json:"flag"`
	Perm       os.FileMode `json:"perm"`
	WithAvatar bool        `json:"withAvatar"` // 是否获取替身文件
}
