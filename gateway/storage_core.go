package gateway

import (
	sdkconst "github.com/siyouyun-open/siyouyun_sdk/const"
	"github.com/siyouyun-open/siyouyun_sdk/pkg/dto"
	"github.com/siyouyun-open/siyouyun_sdk/restclient"
	"github.com/siyouyun-open/siyouyun_sdk/utils"
	"strconv"
)

type storageCoreApi struct {
	Host string
	*utils.UserNamespace
}

var storageCoreGatewayAddr = OSURL + "/fs"

func newStorageCoreApi(un *utils.UserNamespace) *storageCoreApi {
	return &storageCoreApi{
		Host:          storageCoreGatewayAddr,
		UserNamespace: un,
	}
}

// PathToInode path转inode
func (sc storageCoreApi) PathToInode(path string) int64 {
	api := sc.Host + "/path/to/inode"
	response := restclient.PostRequest[int64](sc.UserNamespace, api, map[string]string{"path": path}, nil)
	if response.Code != sdkconst.Success {
		return 0
	}
	return *response.Data
}

// InodeToPath inode转path
func (sc storageCoreApi) InodeToPath(inode int64) string {
	api := sc.Host + "/inode/to/path"
	response := restclient.PostRequest[string](sc.UserNamespace, api, map[string]string{"inode": strconv.FormatInt(inode, 10)}, nil)
	if response.Code != sdkconst.Success {
		return ""
	}
	return *response.Data
}

// InodeToFileInfo inode转fileInfo
func (sc storageCoreApi) InodeToFileInfo(inode int64) *sdkdto.FileInfoRes {
	api := sc.Host + "/file/info/by/inode"
	response := restclient.PostRequest[sdkdto.FileInfoRes](sc.UserNamespace, api, map[string]string{"inode": strconv.FormatInt(inode, 10)}, nil)
	if response.Code != sdkconst.Success {
		return nil
	}
	return response.Data
}

// InodesToFileInfos inodes转fileInfos
func (sc storageCoreApi) InodesToFileInfos(inodes ...int64) map[int64]sdkdto.FileInfoRes {
	api := sc.Host + "/file/infos/map/by/inodes"
	response := restclient.PostRequest[map[int64]sdkdto.FileInfoRes](sc.UserNamespace, api, nil, inodes)
	if response.Code != sdkconst.Success {
		return nil
	}
	return *response.Data
}
