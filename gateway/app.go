package gateway

import (
	"fmt"
	"github.com/siyouyun-open/siyouyun_sdk/entity"
)

const (
	rootUser        = ""
	rootPasswd      = ""
	defaultDatabase = "siyou_common"
	mysqlDSNTmpl    = "%s:%s@tcp(127.0.0.1:3306)/%s?charset=utf8mb4&parseTime=True&loc=Local"
)

func GetAppInfo(code string) *entity.AppRegisterInfo {
	return &entity.AppRegisterInfo{
		AppCode:          "1",
		AppName:          "2",
		AppDesc:          "3",
		AppVersion:       "4",
		DSN:              fmt.Sprintf(mysqlDSNTmpl, rootUser, rootPasswd, defaultDatabase),
		RegisterUserList: []string{"zhangsan"},
	}
}
