package gateway

import (
	"github.com/siyouyun-open/siyouyun_sdk/pkg/dto"
	"github.com/siyouyun-open/siyouyun_sdk/pkg/utils"
	"os"
	"time"
)

type StorageApi struct {
	*storageOSApi
	*storageCoreApi
}

func NewStorageApi(ugn *utils.UserGroupNamespace) *StorageApi {
	return &StorageApi{
		storageOSApi:   newStorageOSApi(ugn),
		storageCoreApi: newStorageCoreApi(ugn),
	}
}

// Open  打开文件
func (s StorageApi) Open(path string) (*os.File, error) {
	return s.storageOSApi.Open(path)
}

// OpenFile 打开或创建文件
func (s StorageApi) OpenFile(path string, flag int, perm os.FileMode) (*os.File, error) {
	return s.storageOSApi.OpenFile(path, flag, perm)
}

// OpenAvatarFile 打开替身文件
func (s StorageApi) OpenAvatarFile(path string) (*os.File, error) {
	return s.storageOSApi.OpenAvatarFile(path)
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

// Chtimes 修改文件时间
func (s StorageApi) Chtimes(path string, atime time.Time, mtime time.Time) error {
	return s.storageOSApi.Chtimes(path, atime, mtime)
}

// FileExists 文件是否存在
func (s StorageApi) FileExists(path string) bool {
	return s.storageOSApi.FileExists(path)
}

// EnsureDirExist 确保目录存在
func (s StorageApi) EnsureDirExist(path string) {
	s.storageOSApi.EnsureDirExist(path)
}

// PathToInode path转inode
func (s StorageApi) PathToInode(path string) uint64 {
	return s.storageCoreApi.PathToInode(path)
}

// InodeToPath inode转path
func (s StorageApi) InodeToPath(inode uint64) string {
	return s.storageCoreApi.InodeToPath(inode)
}

// InodeToFileInfo inode转fileInfo
func (s StorageApi) InodeToFileInfo(inode uint64) *sdkdto.FileInfoRes {
	return s.storageCoreApi.InodeToFileInfo(inode)
}

// InodesToFileInfos inodes转fileInfos
func (s StorageApi) InodesToFileInfos(inodes ...uint64) map[uint64]*sdkdto.FileInfoRes {
	return s.storageCoreApi.InodesToFileInfos(inodes...)
}
