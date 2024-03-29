package siyouyunsdk

import (
	"gorm.io/gorm"
	"os"
)

type EventFS struct {
	FileEvent *FileEvent
	FS        *FS
	AppFS     *AppFS
	*Ability
}

func (a *AppStruct) newEventFSFromFileEvent(fe *FileEvent) *EventFS {
	efs := &EventFS{
		FileEvent: fe,
		FS:        a.NewFSFromUserGroupNamespace(fe.UGN),
		AppFS:     a.NewAppFSFromUserGroupNamespace(fe.UGN),
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

// OpenEventFile  open event's file
func (efs *EventFS) OpenEventFile() (*os.File, error) {
	path := efs.FS.InodeToPath(efs.FileEvent.Inode)
	return efs.FS.Open(path)
}

// OpenAvatarFile open event's avatar file
func (efs *EventFS) OpenAvatarFile() (*os.File, error) {
	return efs.FS.OpenAvatarFile(efs.FileEvent.FullPath)
}

func (efs *EventFS) Destroy() {
	efs.FS.Destroy()
	efs.AppFS.Destroy()
}

func (efs *EventFS) Exec(f func(*gorm.DB) error) error {
	return efs.FS.Exec(f)
}
