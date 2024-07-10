package ability

import (
	"errors"
	sdkconst "github.com/siyouyun-open/siyouyun_sdk/pkg/const"
	"github.com/siyouyun-open/siyouyun_sdk/pkg/dto"
	"github.com/siyouyun-open/siyouyun_sdk/pkg/restclient"
	"github.com/siyouyun-open/siyouyun_sdk/pkg/utils"
)

type FFmpeg struct {
	gatewayAddr string
}

func NewFFmpeg() *FFmpeg {
	return &FFmpeg{
		gatewayAddr: sdkconst.OSURL + "/codec",
	}
}

func (ff *FFmpeg) Name() string {
	return "FFmpeg"
}

func (ff *FFmpeg) Close() {
}

func (ff *FFmpeg) GetBasicInfo(ugn *utils.UserGroupNamespace, parentPath, name string) (*sdkdto.FFmpegBasicInfo, error) {
	info, err := ff.getInfo(ugn, parentPath, name)
	if err != nil {
		return nil, err
	}
	return &sdkdto.FFmpegBasicInfo{
		Duration: info.Format.Duration,
		Size:     info.Format.Size,
		BitRate:  info.Format.BitRate,
	}, nil
}

func (ff *FFmpeg) GetDetailInfo(ugn *utils.UserGroupNamespace, parentPath, name string) (*sdkdto.FFProbeInfo, error) {
	return ff.getInfo(ugn, parentPath, name)
}

func (ff *FFmpeg) getInfo(ugn *utils.UserGroupNamespace, parentPath, name string) (*sdkdto.FFProbeInfo, error) {
	api := ff.gatewayAddr + "/ffmpeg/info"
	response := restclient.GetRequest[sdkdto.FFProbeInfo](
		ugn,
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
