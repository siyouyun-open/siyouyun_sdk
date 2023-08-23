package siyouyunsdk

import (
	"gorm.io/gorm"
)

type EventFS struct {
	EventFileInode int64
	EventFilePath  string
	FS             *FS
	AppFS          *AppFS
	*Ability
}

func (a *AppStruct) newEventFSFromFileEvent(fe *FileEvent) *EventFS {
	efs := &EventFS{
		EventFileInode: fe.Inode,
		FS:             a.NewFSFromUserNamespace(&fe.UGN),
		AppFS:          a.NewAppFSFromUserNamespace(&fe.UGN),
	}
	efs.Ability = efs.FS.Ability
	return efs
}

func (a *AppStruct) newEventFSFromScheduleEvent(se *ScheduleEvent) *EventFS {
	efs := &EventFS{
		FS:    a.NewFSFromUserNamespace(&se.UGN),
		AppFS: a.NewAppFSFromUserNamespace(&se.UGN),
	}
	efs.Ability = efs.FS.Ability
	return efs
}

// OpenEventFile  打开事件相关文件
func (efs *EventFS) OpenEventFile() (*SyyFile, error) {
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
