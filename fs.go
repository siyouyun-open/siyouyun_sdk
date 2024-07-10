package siyouyunsdk

import (
	"github.com/kataras/iris/v12"
	"github.com/siyouyun-open/siyouyun_sdk/internal/gateway"
	"github.com/siyouyun-open/siyouyun_sdk/pkg/dto"
	"github.com/siyouyun-open/siyouyun_sdk/pkg/utils"
	"gorm.io/gorm"
	"os"
	"time"
)

// FS fs
type FS struct {
	api *gateway.StorageApi
	App *AppStruct
	UGN *utils.UserGroupNamespace
}

func (a *AppStruct) NewFSFromCtx(ctx iris.Context) *FS {
	ugn := utils.NewUserNamespaceFromIris(ctx)
	fs := &FS{
		api: gateway.NewStorageApi(ugn),
		App: a,
		UGN: ugn,
	}
	return fs
}

func (a *AppStruct) NewFSFromUserGroupNamespace(ugn *utils.UserGroupNamespace) *FS {
	fs := &FS{
		App: a,
		UGN: ugn,
		api: gateway.NewStorageApi(ugn),
	}
	return fs
}

// Open  打开文件
func (fs *FS) Open(path string) (*os.File, error) {
	return fs.api.Open(path)
}

// OpenByInode 根据inode打开文件
func (fs *FS) OpenByInode(inode uint64) (*os.File, error) {
	return fs.Open(fs.InodeToPath(inode))
}

// OpenFile 打开或创建文件
func (fs *FS) OpenFile(path string, flag int, perm os.FileMode) (*os.File, error) {
	return fs.api.OpenFile(path, flag, perm)
}

func (fs *FS) OpenAvatarFile(path string) (*os.File, error) {
	return fs.api.OpenAvatarFile(path)
}

// MkdirAll 递归创建目录
func (fs *FS) MkdirAll(path string) error {
	return fs.api.MkdirAll(path)
}

// Remove 删除文件或空目录
func (fs *FS) Remove(path string) error {
	return fs.api.Remove(path)
}

// RemoveAll 删除文件或文件夹（包括子目录）
func (fs *FS) RemoveAll(path string) error {
	return fs.api.RemoveAll(path)
}

// Rename 重命名文件
func (fs *FS) Rename(oldPath, newPath string) error {
	return fs.api.Rename(oldPath, newPath)
}

// Chtimes 修改文件时间
func (fs *FS) Chtimes(path string, atime time.Time, mtime time.Time) error {
	return fs.api.Chtimes(path, atime, mtime)
}

// FileExists 文件是否存在
func (fs *FS) FileExists(path string) bool {
	return fs.api.FileExists(path)
}

// EnsureDirExist 确保目录存在
func (fs *FS) EnsureDirExist(path string) {
	fs.api.EnsureDirExist(path)
}

// PathToInode path转inode
func (fs *FS) PathToInode(path string) uint64 {
	return fs.api.PathToInode(path)
}

// InodeToPath inode转path
func (fs *FS) InodeToPath(inode uint64) string {
	return fs.api.InodeToPath(inode)
}

// InodeToFileInfo inode转fileInfo
func (fs *FS) InodeToFileInfo(inode uint64) *sdkdto.FileInfoRes {
	return fs.api.InodeToFileInfo(inode)
}

// InodesToFileInfos inodes转fileInfos
func (fs *FS) InodesToFileInfos(inodes ...uint64) map[uint64]*sdkdto.FileInfoRes {
	return fs.api.InodesToFileInfos(inodes...)
}

func (fs *FS) Destroy() {
}

// Exec  fs执行sql
func (fs *FS) Exec(f func(*gorm.DB) error) error {
	err := fs.App.db.Transaction(func(tx *gorm.DB) (err error) {
		dbname := fs.UGN.DatabaseName()
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
