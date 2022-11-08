package gateway

import (
	"fmt"
	sdkconst "github.com/siyouyun-open/siyouyun_sdk/const"
	"github.com/siyouyun-open/siyouyun_sdk/pkg/dto"
	"github.com/siyouyun-open/siyouyun_sdk/restclient"
	"github.com/siyouyun-open/siyouyun_sdk/utils"
	"strconv"
	"strings"
)

type storageCoreApi struct {
	Host string
	*utils.UserNamespace
}

var storageCoreGatewayAddr = fmt.Sprintf("%s:%d/%s", LocalhostAddress, CoreHTTPPort, "fs")

func newStorageCoreApi(un *utils.UserNamespace) *storageCoreApi {
	return &storageCoreApi{
		Host:          storageCoreGatewayAddr,
		UserNamespace: un,
	}
}

// PathToInode path转inode
func (sc storageCoreApi) PathToInode(path string) int64 {
	api := sc.Host + "/inode/to/path"
	response := restclient.PostRequest[int64](
		sc.UserNamespace,
		api,
		map[string]string{"path": path},
		nil,
	)
	if response.Code != sdkconst.Success {
		return 0
	}
	return *response.Data
}

// InodeToPath inode转path
func (sc storageCoreApi) InodeToPath(inode int64) string {
	api := sc.Host + "/path/to/inode"
	response := restclient.PostRequest[string](
		sc.UserNamespace,
		api,
		map[string]string{"inode": strconv.FormatInt(inode, 10)},
		nil,
	)
	if response.Code != sdkconst.Success {
		return ""
	}
	return *response.Data
}

// InodeToFileInfo inode转fileInfo
func (sc storageCoreApi) InodeToFileInfo(inode int64) *dto.FileInfoRes {
	api := sc.Host + "/inode/to/fileinfo"
	response := restclient.PostRequest[dto.FileInfoRes](
		sc.UserNamespace,
		api,
		map[string]string{"inode": strconv.FormatInt(inode, 10)},
		nil,
	)
	if response.Code != sdkconst.Success {
		return nil
	}
	return response.Data
}

// InodesToFileInfos inodes转fileInfos
func (sc storageCoreApi) InodesToFileInfos(inodes ...int64) map[int64]dto.FileInfoRes {
	var inodesStr []string
	for i := range inodes {
		inodesStr = append(inodesStr, strconv.FormatInt(inodes[i], 10))
	}
	api := sc.Host + "/inodes/to/fileinfos"
	response := restclient.PostRequest[map[int64]dto.FileInfoRes](
		sc.UserNamespace,
		api,
		map[string]string{
			"inodes": strings.Join(inodesStr, ","),
		},
		nil,
	)
	if response.Code != sdkconst.Success {
		return nil
	}
	return *response.Data
}
