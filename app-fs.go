package siyouyunsdk

import (
	"github.com/kataras/iris/v12"
	"github.com/siyouyun-open/siyouyun_sdk/pkg/dto"
	"github.com/siyouyun-open/siyouyun_sdk/pkg/utils"
	"gorm.io/gorm"
	"os"
	"path/filepath"
	"time"
)

const (
	SiyouyunPrefix = "/.siyouyun"
	AppPrefix      = SiyouyunPrefix + "/appdata"
)

type AppFS struct {
	appNormalPath string
	fs            *FS
	*Ability
}

func (a *AppStruct) NewAppFSFromCtx(ctx iris.Context) *AppFS {
	afs := &AppFS{
		fs: a.NewFSFromCtx(ctx),
	}
	afs.appNormalPath = afs.getNormalAppPrefix()
	afs.Ability = afs.fs.Ability
	return afs
}

func (a *AppStruct) NewAppFSFromUserGroupNamespace(ugn *utils.UserGroupNamespace) *AppFS {
	afs := &AppFS{
		fs: a.NewFSFromUserGroupNamespace(ugn),
	}
	afs.appNormalPath = afs.getNormalAppPrefix()
	afs.Ability = afs.fs.Ability
	return afs
}

func (afs *AppFS) getNormalAppPrefix() string {
	return filepath.Join(AppPrefix, afs.fs.App.AppCode)
}

// Open  打开文件
func (afs *AppFS) Open(path string) (*os.File, error) {
	return afs.fs.Open(filepath.Join(afs.appNormalPath, path))
}

// OpenFile 打开或创建文件
func (afs *AppFS) OpenFile(path string, flag int, perm os.FileMode) (*os.File, error) {
	return afs.fs.OpenFile(filepath.Join(afs.appNormalPath, path), flag, perm)
}

// MkdirAll 递归创建目录
func (afs *AppFS) MkdirAll(path string) error {
	return afs.fs.MkdirAll(filepath.Join(afs.appNormalPath, path))
}

// Remove 删除文件
func (afs *AppFS) Remove(path string) error {
	return afs.fs.Remove(filepath.Join(afs.appNormalPath, path))
}

// Rename 重命名文件
func (afs *AppFS) Rename(oldPath, newPath string) error {
	return afs.fs.Rename(filepath.Join(afs.appNormalPath, oldPath), filepath.Join(afs.appNormalPath, newPath))
}

// Chtimes 修改文件时间
func (afs *AppFS) Chtimes(path string, atime time.Time, mtime time.Time) error {
	return afs.fs.Chtimes(filepath.Join(afs.appNormalPath, path), atime, mtime)
}

// FileExists 文件是否存在
func (afs *AppFS) FileExists(path string) bool {
	return afs.fs.FileExists(filepath.Join(afs.appNormalPath, path))
}

// EnsureDirExist 确保目录存在
func (afs *AppFS) EnsureDirExist(ps ...string) {
	var nps []string
	for i := range ps {
		nps = append(nps, filepath.Join(afs.appNormalPath, ps[i]))
	}
	afs.fs.EnsureDirExist(nps...)
}

// PathToInode path转inode
func (afs *AppFS) PathToInode(path string) uint64 {
	return afs.fs.PathToInode(filepath.Join(afs.appNormalPath, path))
}

// InodeToPath inode转path
func (afs *AppFS) InodeToPath(inode uint64) string {
	return afs.fs.InodeToPath(inode)
}

// InodeToFileInfo inode转fileInfo
func (afs *AppFS) InodeToFileInfo(inode uint64) *sdkdto.FileInfoRes {
	return afs.fs.InodeToFileInfo(inode)
}

// InodesToFileInfos inodes转fileInfos
func (afs *AppFS) InodesToFileInfos(inodes ...uint64) map[uint64]*sdkdto.FileInfoRes {
	return afs.fs.InodesToFileInfos(inodes...)
}

func (afs *AppFS) Destroy() {
	afs.fs.Destroy()
}

func (afs *AppFS) Exec(f func(*gorm.DB) error) error {
	return afs.fs.Exec(f)
}

func (afs *AppFS) GetUGN() *utils.UserGroupNamespace {
	return afs.fs.UGN
}
