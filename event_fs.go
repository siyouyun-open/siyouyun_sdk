package siyouyunfaas

import (
	"github.com/siyouyun-open/siyouyun_sdk/const"
	"github.com/siyouyun-open/siyouyun_sdk/gateway"
	"github.com/siyouyun-open/siyouyun_sdk/utils"
	"os"
	"path/filepath"
	"strings"
)

type EventFS struct {
	eventFileInode int64
	appNormalPath  string
	fs             *FS
}

func (a *App) newEventFSFromFileEvent(fe *FileEvent) *EventFS {
	un := &utils.UserNamespace{
		Username:  fe.Username,
		Namespace: fe.Namespace,
	}
	efs := &EventFS{
		eventFileInode: fe.Inode,
		fs:             a.NewFSFromUserNamespace(un),
	}
	efs.appNormalPath = efs.getNormalAppPrefix()
	return efs
}

func (efs *EventFS) getNormalAppPrefix() string {
	var prefixPath string
	switch efs.fs.Namespace {
	case "":
		fallthrough
	case sdkconst.MainNamespace:
		prefixPath = filepath.Join(
			sdkconst.FaasMntPrefix,
			efs.fs.Username,
			strings.Join([]string{efs.fs.Username, sdkconst.MainNamespace}, "-"),
			".siyouyun", "app",
			efs.fs.AppCodeName,
		)
	case sdkconst.PrivateNamespace:
		prefixPath = filepath.Join(
			sdkconst.FaasMntPrefix,
			efs.fs.Username,
			strings.Join([]string{efs.fs.Username, sdkconst.PrivateNamespace}, "-"),
			".siyouyun", "app",
			efs.fs.AppCodeName,
		)
	case sdkconst.CommonNamespace:
		prefixPath = filepath.Join(
			sdkconst.FaasMntPrefix,
			sdkconst.CommonNamespace,
			".siyouyun", "app",
			efs.fs.AppCodeName,
		)
	}
	return prefixPath
}

// OpenEventFile  打开事件相关文件
func (efs *EventFS) OpenEventFile() (*os.File, error) {
	path := efs.fs.InodeToPath(efs.eventFileInode)
	return efs.fs.Open(path)
}

// Open  打开文件
func (efs *EventFS) Open(path string) (*os.File, error) {
	return efs.fs.Open(filepath.Join(efs.appNormalPath, path))
}

// OpenFile 打开或创建文件
func (efs *EventFS) OpenFile(path string, flag int, perm os.FileMode) (*os.File, error) {
	return efs.fs.OpenFile(filepath.Join(efs.appNormalPath, path), flag, perm)
}

// MkdirAll 递归创建目录
func (efs *EventFS) MkdirAll(path string) error {
	return efs.fs.MkdirAll(filepath.Join(efs.appNormalPath, path))
}

// Remove 删除文件
func (efs *EventFS) Remove(path string) error {
	return efs.fs.Remove(filepath.Join(efs.appNormalPath, path))
}

// Rename 重命名文件
func (efs *EventFS) Rename(oldPath, newPath string) error {
	return efs.fs.Rename(filepath.Join(efs.appNormalPath, oldPath), filepath.Join(efs.appNormalPath, newPath))
}

// FileExists 文件是否存在
func (efs *EventFS) FileExists(path string) bool {
	return efs.fs.FileExists(filepath.Join(efs.appNormalPath, path))
}

// EnsureDirExist 确保目录存在
func (efs *EventFS) EnsureDirExist(ps ...string) {
	var nps []string
	for i := range ps {
		nps = append(nps, filepath.Join(efs.appNormalPath, ps[i]))
	}
	efs.fs.EnsureDirExist(nps...)
}

// PathToInode path转inode
func (efs *EventFS) PathToInode(path string) int64 {
	return efs.fs.PathToInode(filepath.Join(efs.appNormalPath, path))
}

// InodeToPath inode转path
func (efs *EventFS) InodeToPath(inode int64) string {
	return efs.fs.InodeToPath(inode)
}

// InodeToFileInfo inode转fileInfo
func (efs *EventFS) InodeToFileInfo(inode int64) *gateway.FileInfoRes {
	return efs.fs.InodeToFileInfo(inode)
}

// InodesToFileInfos inodes转fileInfos
func (efs *EventFS) InodesToFileInfos(inodes ...int64) map[string]gateway.FileInfoRes {
	return efs.fs.InodesToFileInfos(inodes...)
}
