package ability

import (
	"github.com/siyouyun-open/siyouyun_sdk/internal/gateway"
	sdkdto "github.com/siyouyun-open/siyouyun_sdk/pkg/dto"
	"github.com/siyouyun-open/siyouyun_sdk/pkg/utils"
	"gorm.io/gorm"
	"os"
	"path/filepath"
	"time"
)

type SyyFS struct {
	ugn  *utils.UserGroupNamespace
	root string
	db   *gorm.DB
	api  *gateway.StorageApi
}

func (fs *SyyFS) realpath(path string) string {
	return filepath.Join(fs.root, path)
}

// Open  打开文件
func (fs *SyyFS) Open(path string) (*os.File, error) {
	return fs.api.Open(fs.realpath(path))
}

// OpenByInode 根据inode打开文件
func (fs *SyyFS) OpenByInode(inode uint64) (*os.File, error) {
	return fs.Open(fs.InodeToPath(inode))
}

// OpenFile 打开或创建文件
func (fs *SyyFS) OpenFile(path string, flag int, perm os.FileMode) (*os.File, error) {
	return fs.api.OpenFile(fs.realpath(path), flag, perm)
}

func (fs *SyyFS) OpenAvatarFile(path string) (*os.File, error) {
	return fs.api.OpenAvatarFile(fs.realpath(path))
}

// MkdirAll 递归创建目录
func (fs *SyyFS) MkdirAll(path string) error {
	return fs.api.MkdirAll(fs.realpath(path))
}

// Remove 删除文件或空目录
func (fs *SyyFS) Remove(path string) error {
	return fs.api.Remove(fs.realpath(path))
}

// RemoveAll 删除文件或文件夹（包括子目录）
func (fs *SyyFS) RemoveAll(path string) error {
	return fs.api.RemoveAll(fs.realpath(path))
}

// Rename 重命名文件
func (fs *SyyFS) Rename(oldPath, newPath string) error {
	return fs.api.Rename(fs.realpath(oldPath), fs.realpath(newPath))
}

// Chtimes 修改文件时间
func (fs *SyyFS) Chtimes(path string, atime time.Time, mtime time.Time) error {
	return fs.api.Chtimes(fs.realpath(path), atime, mtime)
}

// FileExists 文件是否存在
func (fs *SyyFS) FileExists(path string) bool {
	return fs.api.FileExists(fs.realpath(path))
}

// EnsureDirExist 确保目录存在
func (fs *SyyFS) EnsureDirExist(path string) {
	fs.api.EnsureDirExist(fs.realpath(path))
}

// PathToInode path转inode
func (fs *SyyFS) PathToInode(path string) uint64 {
	return fs.api.PathToInode(fs.realpath(path))
}

// InodeToPath inode转path
func (fs *SyyFS) InodeToPath(inode uint64) string {
	return fs.api.InodeToPath(inode)
}

// InodeToFileInfo inode转fileInfo
func (fs *SyyFS) InodeToFileInfo(inode uint64) *sdkdto.FileInfoRes {
	return fs.api.InodeToFileInfo(inode)
}

// InodesToFileInfos inodes转fileInfos
func (fs *SyyFS) InodesToFileInfos(inodes ...uint64) map[uint64]*sdkdto.FileInfoRes {
	return fs.api.InodesToFileInfos(inodes...)
}

func (fs *SyyFS) Destroy() {
}

// Exec  fs执行sql
func (fs *SyyFS) Exec(f func(*gorm.DB) error) error {
	err := fs.db.Transaction(func(tx *gorm.DB) (err error) {
		dbname := fs.ugn.DatabaseName()
		if dbname == "" {
			return
		}
		err = tx.Exec("use " + dbname).Error
		if err != nil {
			return err
		}
		err = f(tx)
		if err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return err
	}
	return nil
}
