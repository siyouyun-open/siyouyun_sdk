package ability

import (
	sdkdto "github.com/siyouyun-open/siyouyun_sdk/pkg/dto"
	"gorm.io/gorm"
	"io"
	"os"
	"time"
)

type GenericFS interface {
	Open(ufi string) (File, error)
	OpenFile(ufi string, flag int, perm os.FileMode) (File, error)
	OpenAvatarFile(ufi string) (File, error)
	MkdirAll(ufi string) error
	Remove(ufi string) error
	RemoveAll(ufi string) error
	Rename(oldUFI string, newUFI string) error
	Chtimes(ufi string, atime time.Time, mtime time.Time) error
	FileExists(ufi string) bool
	Exec(f func(*gorm.DB) error) error
	AppOpenFile(path string, flag int, perm os.FileMode) (File, error)
	AppMkdirAll(path string) error
	AppRemoveAll(path string) error
	AppFileExists(path string) bool
}

type File interface {
	io.Closer
	io.Reader
	io.ReaderAt
	io.Seeker
	io.Writer
	io.WriterAt

	Name() string
	Readdir(count int) ([]*sdkdto.SiyouFileBasicInfo, error)
	Readdirnames(n int) ([]string, error)
	Stat() (*sdkdto.SiyouFileBasicInfo, error)
	Sync() error
	Truncate(size int64) error
	WriteString(s string) (ret int, err error)
}
