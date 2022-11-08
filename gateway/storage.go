package gateway

import (
	"github.com/siyouyun-open/siyouyun_sdk/utils"
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

type FileInfoRes struct {
	Id           int64       `json:"id"`
	HasThumbnail bool        `json:"hasThumbnail"`
	Name         string      `json:"name"`
	Size         int64       `json:"size"`
	ParentPath   string      `json:"parentPath"`
	FullPath     string      `json:"fullPath"`
	IsDir        bool        `json:"isDir"`
	Tag          string      `json:"tag"`
	Md5          string      `json:"md5"`
	Extension    string      `json:"extension"`
	Mime         string      `json:"mime"`
	Owner        string      `json:"owner"`
	Atime        int64       `json:"atime"`
	Mtime        int64       `json:"mtime"`
	Ctime        int64       `json:"ctime"`
	Tags         interface{} `json:"tags"`
	Ext0         interface{} `json:"ext0"`
	Ext1         interface{} `json:"ext1"`
	Ext2         interface{} `json:"ext2"`

	EventList interface{} `json:"eventList,omitempty"`
}

// Open  打开文件
func (s StorageApi) Open(path string) (*os.File, error) {
	return s.storageOSApi.Open(path)
}

// OpenFile 打开或创建文件
func (s StorageApi) OpenFile(path string, flag int, perm os.FileMode) (*os.File, error) {
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
func (s StorageApi) InodeToFileInfo(inode int64) *FileInfoRes {
	return s.storageCoreApi.InodeToFileInfo(inode)
}

// InodesToFileInfos inodes转fileInfos
func (s StorageApi) InodesToFileInfos(inodes ...int64) map[string]FileInfoRes {
	return s.storageCoreApi.InodesToFileInfos(inodes...)
}
