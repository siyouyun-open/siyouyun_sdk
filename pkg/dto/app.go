package sdkdto

import (
	"github.com/siyouyun-open/siyouyun_sdk/pkg/utils"
)

// AppRegisterInfo app registry Info
type AppRegisterInfo struct {
	AppCode string                     `json:"appCode"`
	AppName string                     `json:"appName"`
	AppDSN  string                     `json:"appDSN"`
	AppAddr string                     `json:"appAddr"`
	UGNList []utils.UserGroupNamespace `json:"ugnList"`
}

// AppDataStatus app data status
type AppDataStatus struct {
	CurrentVersion int  `json:"currentVersion"`
	LatestVersion  int  `json:"latestVersion"`
	NeedRefresh    bool `json:"needRefresh"`
}
