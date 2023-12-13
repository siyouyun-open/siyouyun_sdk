package siyouyunsdk

import (
	"gorm.io/gorm"
	"os"
)

type EventFS struct {
	EventFileInode uint64
	EventFilePath  string
	FS             *FS
	AppFS          *AppFS
	*Ability
}

func (a *AppStruct) newEventFSFromFileEvent(fe *FileEvent) *EventFS {
	efs := &EventFS{
		EventFileInode: fe.Inode,
		EventFilePath:  fe.FullPath,
		FS:             a.NewFSFromUserGroupNamespace(fe.UGN),
		AppFS:          a.NewAppFSFromUserGroupNamespace(fe.UGN),
	}
	efs.Ability = efs.FS.Ability
	return efs
}

func (a *AppStruct) newEventFSFromScheduleEvent(se *ScheduleEvent) *EventFS {
	efs := &EventFS{
		FS:    a.NewFSFromUserGroupNamespace(&se.UGN),
		AppFS: a.NewAppFSFromUserGroupNamespace(&se.UGN),
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
