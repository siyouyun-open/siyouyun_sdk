package siyouyunsdk

import (
	"github.com/kataras/iris/v12"
	"github.com/siyouyun-open/siyouyun_sdk/gateway"
	"github.com/siyouyun-open/siyouyun_sdk/pkg/dto"
	"github.com/siyouyun-open/siyouyun_sdk/utils"
	"gorm.io/gorm"
	"os"
	"time"
)

// FS fs
type FS struct {
	AppCodeName string
	*Ability
	api *gateway.StorageApi
	App *AppStruct
	*utils.UserNamespace
}

func (a *AppStruct) NewFSFromCtx(ctx iris.Context) *FS {
	un := utils.NewUserNamespaceFromIris(ctx)
	fs := &FS{
		AppCodeName:   a.AppCode,
		api:           gateway.NewStorageApi(un),
		App:           a,
		UserNamespace: un,
	}
	fs.initAbility()
	return fs
}

func (a *AppStruct) NewFSFromUserNamespace(un *utils.UserNamespace) *FS {
	fs := &FS{
		AppCodeName:   a.AppCode,
		App:           a,
		UserNamespace: un,
		api:           gateway.NewStorageApi(un),
	}
	fs.initAbility()
	return fs
}

func (fs *FS) initAbility() {
	fs.Ability = new(Ability)
	fs.Ability.KV = fs.NewKV()
	fs.Ability.FFmpeg = fs.NewFFmpeg()
	fs.Ability.Schedule = fs.NewSchedule()
	fs.Ability.Message = new(Message)
}

// Open  打开文件
func (fs *FS) Open(path string) (*SyyFile, error) {
	file, conn, usfp, err := fs.api.Open(path)
	return &SyyFile{
		Fullpath:       path,
		file:           file,
		unixConn:       conn,
		unixSocketPath: usfp,
	}, err
}

// OpenFile 打开或创建文件
func (fs *FS) OpenFile(path string, flag int, perm os.FileMode) (*SyyFile, error) {
	file, conn, usfp, err := fs.api.OpenFile(path, flag, perm)
	return &SyyFile{
		file:           file,
		unixConn:       conn,
		unixSocketPath: usfp,
	}, err
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

// Chtimes 修改文件时间
func (fs *FS) Chtimes(path string, atime time.Time, mtime time.Time) error {
	return fs.api.Chtimes(path, atime, mtime)
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
func (fs *FS) InodeToFileInfo(inode int64) *sdkdto.FileInfoRes {
	return fs.api.InodeToFileInfo(inode)
}

// InodesToFileInfos inodes转fileInfos
func (fs *FS) InodesToFileInfos(inodes ...int64) map[int64]sdkdto.FileInfoRes {
	return fs.api.InodesToFileInfos(inodes...)
}

func (fs *FS) Destroy() {
	//for s := range fs.unixConnMap {
	//	if v, ok := fs.unixConnMap[s]; ok {
	//		v.Close()
	//	}
	//	cmd := exec.Command("rm", s)
	//	err := cmd.Run()
	//	if err != nil {
	//	}
	//}
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
