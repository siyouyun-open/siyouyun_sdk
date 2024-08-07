package gateway

import (
	"github.com/siyouyun-open/siyouyun_sdk/pkg/const"
	"github.com/siyouyun-open/siyouyun_sdk/pkg/dto"
	"github.com/siyouyun-open/siyouyun_sdk/pkg/restclient"
	"github.com/siyouyun-open/siyouyun_sdk/pkg/utils"
	"strconv"
)

type storageMetaApi struct {
	Host string
	UGN  *utils.UserGroupNamespace
}

func newStorageMetaApi(ugn *utils.UserGroupNamespace) *storageMetaApi {
	return &storageMetaApi{
		Host: utils.GetOSServiceURL() + "/fs",
		UGN:  ugn,
	}
}

// PathToInode path转inode
func (sc storageMetaApi) PathToInode(path string) uint64 {
	api := sc.Host + "/path/to/inode"
	response := restclient.PostRequest[uint64](sc.UGN, api, map[string]string{"path": path}, nil)
	if response.Code != sdkconst.Success {
		return 0
	}
	return *response.Data
}

// InodeToPath inode转path
func (sc storageMetaApi) InodeToPath(inode uint64) string {
	api := sc.Host + "/inode/to/path"
	response := restclient.PostRequest[string](sc.UGN, api, map[string]string{"inode": strconv.FormatUint(inode, 10)}, nil)
	if response.Code != sdkconst.Success {
		return ""
	}
	return *response.Data
}

// InodeToFileInfo inode转fileInfo
func (sc storageMetaApi) InodeToFileInfo(inode uint64) *sdkdto.FileInfoRes {
	api := sc.Host + "/file/info/by/inode"
	response := restclient.PostRequest[sdkdto.FileInfoRes](sc.UGN, api, map[string]string{"inode": strconv.FormatUint(inode, 10)}, nil)
	if response.Code != sdkconst.Success {
		return nil
	}
	return response.Data
}

// InodesToFileInfos inodes转fileInfos
func (sc storageMetaApi) InodesToFileInfos(inodes ...uint64) map[uint64]*sdkdto.FileInfoRes {
	api := sc.Host + "/file/infos/map/by/inodes"
	response := restclient.PostRequest[map[uint64]*sdkdto.FileInfoRes](sc.UGN, api, nil, inodes)
	if response.Code != sdkconst.Success {
		return nil
	}
	return *response.Data
}
