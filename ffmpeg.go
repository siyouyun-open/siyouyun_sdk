package siyouyunsdk

import (
	"github.com/siyouyun-open/siyouyun_sdk/gateway"
	"github.com/siyouyun-open/siyouyun_sdk/pkg/dto"
)

type FFmpeg struct {
	*gateway.FFmpegOSApi
}

func (fs *FS) NewFFmpeg() *FFmpeg {
	return &FFmpeg{
		FFmpegOSApi: gateway.NewFFmpegOSApi(fs.UserNamespace),
	}
}

func (ff *FFmpeg) GetBasicInfo(parentPath, name string) (*sdkdto.FFmpegBasicInfo, error) {
	info, err := ff.FFmpegOSApi.GetInfo(parentPath, name)
	if err != nil {
		return nil, err
	}
	return &sdkdto.FFmpegBasicInfo{
		Duration: info.Format.Duration,
		Size:     info.Format.Size,
		BitRate:  info.Format.BitRate,
	}, nil
}

func (ff *FFmpeg) GetDetailInfo(parentPath, name string) (*sdkdto.FFProbeInfo, error) {
	return ff.FFmpegOSApi.GetInfo(parentPath, name)
}
