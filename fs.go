package siyouyunfaas

import (
	"github.com/kataras/iris/v12"
	"github.com/siyouyun-open/siyouyun_sdk/gateway"
	"github.com/siyouyun-open/siyouyun_sdk/utils"
	"gorm.io/gorm"
	"os"
)

// FS fs
type FS struct {
	AppCodeName string

	api *gateway.StorageApi
	app *App
	*utils.UserNamespace
}

func (a *App) NewFSFromCtx(ctx iris.Context) *FS {
	un := utils.NewUserNamespaceFromIris(ctx)
	fs := &FS{
		AppCodeName:   a.AppCode,
		app:           a,
		UserNamespace: un,
		api:           gateway.NewStorageApi(un),
	}
	return fs
}

func (a *App) NewFSFromUserNamespace(un *utils.UserNamespace) *FS {
	fs := &FS{
		AppCodeName:   a.AppCode,
		app:           a,
		UserNamespace: un,
		api:           gateway.NewStorageApi(un),
	}
	return fs
}

// Open  打开文件
func (fs *FS) Open(path string) (*os.File, error) {
	return fs.api.Open(path)
}

// OpenFile 打开或创建文件
func (fs *FS) OpenFile(path string, flag int, perm os.FileMode) (*os.File, error) {
	return fs.api.OpenFile(path, flag, perm)
}

// MkdirAll 递归创建目录
func (fs *FS) MkdirAll(path string) error {
	return fs.api.MkdirAll(path)
}

// Remove 删除文件
func (fs *FS) Remove(path string) error {
	return fs.api.Remove(path)
}

// Rename 重命名文件
func (fs *FS) Rename(oldPath, newPath string) error {
	return fs.api.Rename(oldPath, newPath)
}

// FileExists 文件是否存在
func (fs *FS) FileExists(path string) bool {
	return fs.api.FileExists(path)
}

// EnsureDirExist 确保目录存在
func (fs *FS) EnsureDirExist(ps ...string) {
	fs.api.EnsureDirExist(ps...)
}

// PathToInode path转inode
func (fs *FS) PathToInode(path string) int64 {
	return fs.api.PathToInode(path)
}

// InodeToPath inode转path
func (fs *FS) InodeToPath(inode int64) string {
	return fs.api.InodeToPath(inode)
}

// InodeToFileInfo inode转fileInfo
func (fs *FS) InodeToFileInfo(inode int64) *gateway.FileInfoRes {
	return fs.api.InodeToFileInfo(inode)
}

// InodesToFileInfos inodes转fileInfos
func (fs *FS) InodesToFileInfos(inodes ...int64) map[string]gateway.FileInfoRes {
	return fs.api.InodesToFileInfos(inodes...)
}

// Exec  fs执行sql
func (fs *FS) Exec(f func(*gorm.DB) error) error {
	err := fs.app.DB.Transaction(func(tx *gorm.DB) (err error) {
		dbname := fs.UserNamespace.DatabaseName()
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
