package ability

import (
	sdkdto "github.com/siyouyun-open/siyouyun_sdk/pkg/dto"
	"gorm.io/gorm"
	"os"
	"time"
)

type GenericFS interface {
	Open(path string) (*os.File, error)
	OpenByInode(inode uint64) (*os.File, error)
	OpenFile(path string, flag int, perm os.FileMode) (*os.File, error)
	OpenAvatarFile(path string) (*os.File, error)
	MkdirAll(path string) error
	Remove(path string) error
	RemoveAll(path string) error
	Rename(oldPath, newPath string) error
	Chtimes(path string, atime time.Time, mtime time.Time) error
	FileExists(path string) bool
	EnsureDirExist(path string)
	PathToInode(path string) uint64
	InodeToPath(inode uint64) string
	InodeToFileInfo(inode uint64) *sdkdto.FileInfoRes
	InodesToFileInfos(inodes ...uint64) map[uint64]*sdkdto.FileInfoRes
	Destroy()
	Exec(f func(*gorm.DB) error) error
}
