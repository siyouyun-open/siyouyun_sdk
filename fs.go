package siyouyunsdk

import (
	"github.com/kataras/iris/v12"
	"github.com/siyouyun-open/siyouyun_sdk/gateway"
	"github.com/siyouyun-open/siyouyun_sdk/pkg/dto"
	"github.com/siyouyun-open/siyouyun_sdk/utils"
	"gorm.io/gorm"
	"net"
	"os"
)

// FS fs
type FS struct {
	AppCodeName string

	unixConnMap map[string]*net.UnixConn

	api *gateway.StorageApi
	App *AppStruct
	*utils.UserNamespace
}

func (a *AppStruct) NewFSFromCtx(ctx iris.Context) *FS {
	un := utils.NewUserNamespaceFromIris(ctx)
	fs := &FS{
		AppCodeName:   a.AppCode,
		unixConnMap:   make(map[string]*net.UnixConn),
		api:           gateway.NewStorageApi(un),
		App:           a,
		UserNamespace: un,
	}
	return fs
}

func (a *AppStruct) NewFSFromUserNamespace(un *utils.UserNamespace) *FS {
	fs := &FS{
		AppCodeName:   a.AppCode,
		unixConnMap:   make(map[string]*net.UnixConn),
		App:           a,
		UserNamespace: un,
		api:           gateway.NewStorageApi(un),
	}
	return fs
}

// Open  打开文件
func (fs *FS) Open(path string) (*os.File, error) {
	file, conn, usfp, err := fs.api.Open(path)
	if err != nil {
		return nil, err
	}
	fs.unixConnMap[usfp] = conn
	return file, nil
}

// OpenFile 打开或创建文件
func (fs *FS) OpenFile(path string, flag int, perm os.FileMode) (*os.File, error) {
	file, conn, usfp, err := fs.api.OpenFile(path, flag, perm)
	if err != nil {
		return nil, err
	}
	fs.unixConnMap[usfp] = conn
	return file, nil
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
func (fs *FS) InodeToFileInfo(inode int64) *dto.FileInfoRes {
	return fs.api.InodeToFileInfo(inode)
}

// InodesToFileInfos inodes转fileInfos
func (fs *FS) InodesToFileInfos(inodes ...int64) map[string]dto.FileInfoRes {
	return fs.api.InodesToFileInfos(inodes...)
}

// Exec  fs执行sql
func (fs *FS) Exec(f func(*gorm.DB) error) error {
	err := fs.App.DB.Transaction(func(tx *gorm.DB) (err error) {
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
