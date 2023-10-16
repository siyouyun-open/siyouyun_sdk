package siyouinterface

import (
	"github.com/siyouyun-open/siyouyun_sdk/pkg/dto"
	"os"
	"time"
)

type FSApi interface {
	// Open  打开文件
	Open(path string) (*os.File, error)
	// OpenFile 打开或创建文件
	OpenFile(path string, flag int, perm os.FileMode) (*os.File, error)
	// MkdirAll 递归创建目录
	MkdirAll(path string) error
	// Remove 删除文件
	Remove(path string) error
	// Rename 重命名文件
	Rename(oldPath, newPath string) error
	// Chtimes 修改文件时间信息
	Chtimes(name string, atime time.Time, mtime time.Time) error
	// FileExists 文件是否存在
	FileExists(path string) bool
	// EnsureDirExist 确保目录存在
	EnsureDirExist(ps ...string)
	// PathToInode path转inode
	PathToInode(path string) int64
	// InodeToPath inode转path
	InodeToPath(inode int64) string
	// InodeToFileInfo inode转fileInfo
	InodeToFileInfo(inode int64) *sdkdto.FileInfoRes
	// InodesToFileInfos inodes转fileInfos
	InodesToFileInfos(inodes ...int64) map[int64]sdkdto.FileInfoRes
}
