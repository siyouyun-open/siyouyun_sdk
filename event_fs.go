package siyouyunsdk

import (
	"github.com/siyouyun-open/siyouyun_sdk/utils"
	"gorm.io/gorm"
	"os"
)

type EventFS struct {
	EventFileInode int64
	FS             *FS
	AppFS          *AppFS
	*Ability
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
	efs.Ability = efs.FS.Ability
	return efs
}

func (a *AppStruct) newEventFSFromScheduleEvent(se *ScheduleEvent) *EventFS {
	un := &utils.UserNamespace{
		Username:  se.Username,
		Namespace: se.Namespace,
	}
	efs := &EventFS{
		FS:    a.NewFSFromUserNamespace(un),
		AppFS: a.NewAppFSFromUserNamespace(un),
	}
	efs.Ability = efs.FS.Ability
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

func (efs *EventFS) Exec(f func(*gorm.DB) error) error {
	return efs.FS.Exec(f)
}
