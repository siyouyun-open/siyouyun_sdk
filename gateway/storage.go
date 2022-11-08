package gateway

import (
	"github.com/siyouyun-open/siyouyun_sdk/pkg/dto"
	"github.com/siyouyun-open/siyouyun_sdk/utils"
	"net"
	"os"
)

type StorageApi struct {
	*storageOSApi
	*storageCoreApi
}

func NewStorageApi(un *utils.UserNamespace) *StorageApi {
	return &StorageApi{
		storageOSApi:   newStorageOSApi(un),
		storageCoreApi: newStorageCoreApi(un),
	}
}

// Open  打开文件
func (s StorageApi) Open(path string) (*os.File, *net.UnixConn, string, error) {
	return s.storageOSApi.Open(path)
}

// OpenFile 打开或创建文件
func (s StorageApi) OpenFile(path string, flag int, perm os.FileMode) (*os.File, *net.UnixConn, string, error) {
	return s.storageOSApi.OpenFile(path, flag, perm)
}

// MkdirAll 递归创建目录
func (s StorageApi) MkdirAll(path string) error {
	return s.storageOSApi.MkdirAll(path)
}

// Remove 删除文件
func (s StorageApi) Remove(path string) error {
	return s.storageOSApi.Remove(path)
}

// Rename 重命名文件
func (s StorageApi) Rename(oldPath, newPath string) error {
	return s.storageOSApi.Rename(oldPath, newPath)
}

// FileExists 文件是否存在
func (s StorageApi) FileExists(path string) bool {
	return s.storageOSApi.FileExists(path)
}

// EnsureDirExist 确保目录存在
func (s StorageApi) EnsureDirExist(ps ...string) {
	s.storageOSApi.EnsureDirExist(ps...)
}

// PathToInode path转inode
func (s StorageApi) PathToInode(path string) int64 {
	return s.storageCoreApi.PathToInode(path)
}

// InodeToPath inode转path
func (s StorageApi) InodeToPath(inode int64) string {
	return s.storageCoreApi.InodeToPath(inode)
}

// InodeToFileInfo inode转fileInfo
func (s StorageApi) InodeToFileInfo(inode int64) *dto.FileInfoRes {
	return s.storageCoreApi.InodeToFileInfo(inode)
}

// InodesToFileInfos inodes转fileInfos
func (s StorageApi) InodesToFileInfos(inodes ...int64) map[string]dto.FileInfoRes {
	return s.storageCoreApi.InodesToFileInfos(inodes...)
}
