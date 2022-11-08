package siyouyunsdk

import (
	"github.com/siyouyun-open/siyouyun_sdk/utils"
	"os"
)

type EventFS struct {
	EventFileInode int64
	FS             *FS
	AppFS          *AppFS
}

func (a *AppStruct) newEventFSFromFileEvent(fe *FileEvent) *EventFS {
	un := &utils.UserNamespace{
		Username:  fe.Username,
		Namespace: fe.Namespace,
	}
	efs := &EventFS{
		EventFileInode: fe.Inode,
		FS:             a.NewFSFromUserNamespace(un),
		AppFS:          a.NewAppFSFromUserNamespace(un),
	}
	return efs
}

// OpenEventFile  打开事件相关文件
func (efs *EventFS) OpenEventFile() (*os.File, error) {
	path := efs.FS.InodeToPath(efs.EventFileInode)
	return efs.FS.Open(path)
}

func (efs *EventFS) Destroy() {
	efs.FS.Destroy()
	efs.AppFS.Destroy()
}
