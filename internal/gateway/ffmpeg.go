package gateway

import (
	"errors"
	"github.com/siyouyun-open/siyouyun_sdk/pkg/const"
	"github.com/siyouyun-open/siyouyun_sdk/pkg/dto"
	"github.com/siyouyun-open/siyouyun_sdk/pkg/restclient"
	"github.com/siyouyun-open/siyouyun_sdk/pkg/utils"
)

type FFmpegOSApi struct {
	Host string
	*utils.UserGroupNamespace
}

func NewFFmpegOSApi(ugn *utils.UserGroupNamespace) *FFmpegOSApi {
	return &FFmpegOSApi{
		Host:               OSURL + "/codec",
		UserGroupNamespace: ugn,
	}
}

// GetInfo GetInfo
func (kv *FFmpegOSApi) GetInfo(parentPath, name string) (*sdkdto.FFProbeInfo, error) {
	api := kv.Host + "/ffmpeg/info"
	response := restclient.GetRequest[sdkdto.FFProbeInfo](
		kv.UserGroupNamespace,
		api,
		map[string]string{
			"parentPath": parentPath,
			"name":       name,
		},
	)
	if response.Code != sdkconst.Success {
		return nil, errors.New(response.Msg)
	}
	return response.Data, nil
}
