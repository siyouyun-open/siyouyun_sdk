package gateway

import (
	"errors"
	"fmt"
	"github.com/siyouyun-open/siyouyun_sdk/const"
	"github.com/siyouyun-open/siyouyun_sdk/pkg/dto"
	"github.com/siyouyun-open/siyouyun_sdk/restclient"
	"github.com/siyouyun-open/siyouyun_sdk/utils"
)

type FFmpegOSApi struct {
	Host string
	*utils.UserNamespace
}

var ffmpegOSGatewayAddr = fmt.Sprintf("%s:%d/%s", LocalhostAddress, OSHTTPPort, "codec")

func NewFFmpegOSApi(un *utils.UserNamespace) *FFmpegOSApi {
	return &FFmpegOSApi{
		Host:          ffmpegOSGatewayAddr,
		UserNamespace: un,
	}
}

// GetInfo GetInfo
func (kv *FFmpegOSApi) GetInfo(parentPath, name string) (*sdkdto.FFProbeInfo, error) {
	api := kv.Host + "/ffmpeg/info"
	response := restclient.GetRequest[sdkdto.FFProbeInfo](
		kv.UserNamespace,
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
