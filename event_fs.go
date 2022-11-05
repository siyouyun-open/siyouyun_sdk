package siyouyunfaas

import (
	"github.com/siyouyun-open/siyouyun_sdk/const"
	"github.com/siyouyun-open/siyouyun_sdk/utils"
	"os"
	"path/filepath"
	"strings"
)

type FS struct {
	app *App
	*utils.UserNamespace
	EventFileInode int64
	AppCodeName    string
	mntPath        string
	appPath        string
}

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

func newEventFSFromFileEvent(appCodeName string, fe *FileEvent) *FS {
	un := &utils.UserNamespace{
		Username:  fe.Username,
		Namespace: fe.Namespace,
	}
	efs := &FS{
		UserNamespace:  un,
		EventFileInode: fe.Inode,
		AppCodeName:    appCodeName,
	}
	efs.mntPath = efs.getMntPrefix()
	efs.appPath = efs.getAppPrefix()
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

func (efs *FS) getAppPrefix() string {
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

func (efs *FS) Open(path string) (*os.File, error) {
	return os.OpenFile(filepath.Join(efs.mntPath, path), os.O_RDONLY, 0)
}

func (efs *FS) AppOpen(path string) (*os.File, error) {
	return os.OpenFile(filepath.Join(efs.appPath, path), os.O_RDONLY, 0)
}
func (efs *FS) AppOpenFile(path string, flag int, perm os.FileMode) (*os.File, error) {
	return os.OpenFile(filepath.Join(efs.appPath, path), flag, perm)
}
func (efs *FS) AppRemove(path string) error {
	return os.Remove(filepath.Join(efs.appPath, path))
}
func (efs *FS) AppRename(oldPath, newPath string) error {
	return os.Rename(filepath.Join(efs.appPath, oldPath), filepath.Join(efs.appPath, newPath))
}
