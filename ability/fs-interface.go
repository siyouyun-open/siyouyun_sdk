package ability

import (
	sdkdto "github.com/siyouyun-open/siyouyun_sdk/pkg/dto"
	"github.com/siyouyun-open/siyouyun_sdk/pkg/utils"
	"gorm.io/gorm"
	"io"
	"os"
	"time"
)

type GenericFS interface {
	Open(ufi *utils.UFI) (File, error)
	Open2(ufiStr string) (File, error)
	OpenFile(ufi *utils.UFI, flag int, perm os.FileMode) (File, error)
	OpenAvatarFile(ufi *utils.UFI) (File, error)
	MkdirAll(ufi *utils.UFI) error
	Remove(ufi *utils.UFI) error
	Remove2(ufiStr string) error
	RemoveAll(ufi *utils.UFI) error
	RemoveAll2(ufiStr string) error
	Rename(oldUFI *utils.UFI, newUFI *utils.UFI) error
	Chtimes(ufi *utils.UFI, atime time.Time, mtime time.Time) error
	Chtimes2(ufiStr string, atime time.Time, mtime time.Time) error
	FileExists(ufi *utils.UFI) bool
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
