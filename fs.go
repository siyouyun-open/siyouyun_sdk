package siyouyunfaas

import (
	"errors"
	"github.com/kataras/iris/v12"
	"github.com/siyouyun-open/siyouyun_sdk/const"
	"github.com/siyouyun-open/siyouyun_sdk/utils"
	"os"
	"path/filepath"
	"strings"
)

// FS 事件fs
type FS struct {
	*utils.UserNamespace
	EventFileInode int64
	AppCodeName    string

	app           *App
	mntPath       string
	appNormalPath string
}

// Edge 文件树
type Edge struct {
	Parent         int64
	Name           string
	Inode          int64
	PosixType      int
	MimeGroup      string
	MimeDetail     string
	FullPath       string
	FullParentPath string
}

func (a *App) NewFSFromCtx(ctx iris.Context) *FS {
	un := utils.NewUserNamespaceFromIris(ctx)
	fs := &FS{
		UserNamespace: un,
		AppCodeName:   a.AppCode,
		app:           a,
	}
	fs.mntPath = fs.getMntPrefix()
	fs.appNormalPath = fs.getNormalAppPrefix()
	return fs
}

func (a *App) newEventFSFromFileEvent(fe *FileEvent) *FS {
	un := &utils.UserNamespace{
		Username:  fe.Username,
		Namespace: fe.Namespace,
	}
	efs := &FS{
		UserNamespace:  un,
		EventFileInode: fe.Inode,
		AppCodeName:    a.AppCode,
		app:            a,
	}
	efs.mntPath = efs.getMntPrefix()
	efs.appNormalPath = efs.getNormalAppPrefix()
	return efs
}

func (efs *FS) getMntPrefix() string {
	var prefixPath string
	switch efs.Namespace {
	case "":
		fallthrough
	case sdkconst.MainNamespace:
		prefixPath = filepath.Join(
			sdkconst.FaasMntPrefix,
			efs.Username,
			strings.Join([]string{efs.Username, sdkconst.MainNamespace}, "-"),
		)
	case sdkconst.PrivateNamespace:
		prefixPath = filepath.Join(
			sdkconst.FaasMntPrefix,
			efs.Username,
			strings.Join([]string{efs.Username, sdkconst.PrivateNamespace}, "-"),
		)
	case sdkconst.CommonNamespace:
		prefixPath = filepath.Join(sdkconst.FaasMntPrefix, sdkconst.CommonNamespace)
	}
	return prefixPath
}

func (efs *FS) getNormalAppPrefix() string {
	var prefixPath string
	switch efs.Namespace {
	case "":
		fallthrough
	case sdkconst.MainNamespace:
		prefixPath = filepath.Join(
			sdkconst.FaasMntPrefix,
			efs.Username,
			strings.Join([]string{efs.Username, sdkconst.MainNamespace}, "-"),
			".siyouyun",
			efs.AppCodeName,
		)
	case sdkconst.PrivateNamespace:
		prefixPath = filepath.Join(
			sdkconst.FaasMntPrefix,
			efs.Username,
			strings.Join([]string{efs.Username, sdkconst.PrivateNamespace}, "-"),
			".siyouyun",
			efs.AppCodeName,
		)
	case sdkconst.CommonNamespace:
		prefixPath = filepath.Join(
			sdkconst.FaasMntPrefix,
			sdkconst.CommonNamespace,
			".siyouyun",
			efs.AppCodeName,
		)
	}
	return prefixPath
}

// Open 只读权限打开用户空间文件
func (efs *FS) Open(path string) (*os.File, error) {
	return os.OpenFile(filepath.Join(efs.mntPath, path), os.O_RDONLY, 0)
}

// AppOpen 打开app存储空间文件
func (efs *FS) AppOpen(path string) (*os.File, error) {
	return os.OpenFile(filepath.Join(efs.appNormalPath, path), os.O_RDONLY, 0)
}

// AppMkdir 在app存储空间文件创建目录
func (efs *FS) AppMkdir(path string) error {
	return os.MkdirAll(filepath.Join(efs.appNormalPath, path), os.ModePerm)
}

// AppOpenFile 打开或创建app存储空间文件
func (efs *FS) AppOpenFile(path string, flag int, perm os.FileMode) (*os.File, error) {
	return os.OpenFile(filepath.Join(efs.appNormalPath, path), flag, perm)
}

// AppRemove 删除app存储空间文件
func (efs *FS) AppRemove(path string) error {
	return os.Remove(filepath.Join(efs.appNormalPath, path))
}

// AppRename 重命名app存储空间文件
func (efs *FS) AppRename(oldPath, newPath string) error {
	return os.Rename(filepath.Join(efs.appNormalPath, oldPath), filepath.Join(efs.appNormalPath, newPath))
}

func EnsureDirExist(ps ...string) {
	for _, p := range ps {
		err := os.MkdirAll(p, os.ModePerm)
		if err != nil && !errors.Is(err, os.ErrExist) {
			return
		}
	}
}
