package sdkdto

import (
	"crypto/md5"
	"fmt"

	sdkconst "github.com/siyouyun-open/siyouyun_sdk/pkg/const"
	"github.com/siyouyun-open/siyouyun_sdk/pkg/utils"
)

// FileEvent file event structure
type FileEvent struct {
	UGN        *utils.UserGroupNamespace `json:"ugn"`
	SrcUFI     string                    `json:"srcUfi"` // rename ufi
	UFI        string                    `json:"ufi"`
	Inode      uint64                    `json:"inode"`
	Action     int                       `json:"action"`
	WithAvatar bool                      `json:"withAvatar"`
}

// PreferOptions file event prefer options
type PreferOptions struct {
	MediaType     sdkconst.MediaType                                   `json:"mediaType"`     // media type
	FileEventType int                                                  `json:"fileEventType"` // file event type
	FollowDirs    []string                                             `json:"followDirs"`    // app default follow dirs ("/Photos" represents the dir in the syspool, "*" represents all dir.)
	ExcludeExts   []string                                             `json:"excludeExts"`   // excluded file exts
	Description   string                                               `json:"description"`   // description
	Priority      sdkconst.TaskLevel                                   `json:"priority"`      // priority (resource occupancy level)
	Handler       func(fe *FileEvent) (sdkconst.ConsumeStatus, string) `json:"-"`             // handler
}

func (p *PreferOptions) ParseToEventCode(appCode string) string {
	return fmt.Sprintf("%x", md5.Sum([]byte(fmt.Sprintf("%v%v%v", appCode, p.FileEventType, p.MediaType))))
}

// UserAppEventConfig user app event config
type UserAppEventConfig struct {
	AppCode        string   `json:"appCode"`        // app code
	FollowDirs     []string `json:"followDirs"`     // app default follow dirs
	UserFollowDirs []string `json:"userFollowDirs"` // app user follow dirs
}
