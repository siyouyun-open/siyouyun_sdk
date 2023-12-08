package gateway

import (
	"github.com/siyouyun-open/siyouyun_sdk/pkg/const"
	"github.com/siyouyun-open/siyouyun_sdk/pkg/dto"
	"github.com/siyouyun-open/siyouyun_sdk/pkg/restclient"
	"github.com/siyouyun-open/siyouyun_sdk/pkg/utils"
	"strconv"
)

type storageCoreApi struct {
	Host string
	UGN  *utils.UserGroupNamespace
}

var storageCoreGatewayAddr = OSURL + "/fs"

func newStorageCoreApi(ugn *utils.UserGroupNamespace) *storageCoreApi {
	return &storageCoreApi{
		Host: storageCoreGatewayAddr,
		UGN:  ugn,
	}
}

// PathToInode path转inode
func (sc storageCoreApi) PathToInode(path string) int64 {
	api := sc.Host + "/path/to/inode"
	response := restclient.PostRequest[int64](sc.UGN, api, map[string]string{"path": path}, nil)
	if response.Code != sdkconst.Success {
		return 0
	}
	return *response.Data
}

// InodeToPath inode转path
func (sc storageCoreApi) InodeToPath(inode int64) string {
	api := sc.Host + "/inode/to/path"
	response := restclient.PostRequest[string](sc.UGN, api, map[string]string{"inode": strconv.FormatInt(inode, 10)}, nil)
	if response.Code != sdkconst.Success {
		return ""
	}
	return *response.Data
}

// InodeToFileInfo inode转fileInfo
func (sc storageCoreApi) InodeToFileInfo(inode int64) *sdkdto.FileInfoRes {
	api := sc.Host + "/file/info/by/inode"
	response := restclient.PostRequest[sdkdto.FileInfoRes](sc.UGN, api, map[string]string{"inode": strconv.FormatInt(inode, 10)}, nil)
	if response.Code != sdkconst.Success {
		return nil
	}
	return response.Data
}

// InodesToFileInfos inodes转fileInfos
func (sc storageCoreApi) InodesToFileInfos(inodes ...int64) map[int64]*sdkdto.FileInfoRes {
	api := sc.Host + "/file/infos/map/by/inodes"
	response := restclient.PostRequest[map[int64]*sdkdto.FileInfoRes](sc.UGN, api, nil, inodes)
	if response.Code != sdkconst.Success {
		return nil
	}
	return *response.Data
}
