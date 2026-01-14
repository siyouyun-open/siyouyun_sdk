package ability

import (
	"io"
	"os"
	"time"

	"gorm.io/gorm"

	sdkdto "github.com/siyouyun-open/siyouyun_sdk/pkg/dto"
	"github.com/siyouyun-open/siyouyun_sdk/pkg/utils"
)

type GenericFS interface {
	// GetUGN gets ugn from fs
	GetUGN() *utils.UserGroupNamespace
	// GetDB gets db instance from fs
	GetDB() *gorm.DB
	// Open opens file by ufi
	Open(ufi string) (File, error)
	// OpenFile opens file by ufi, flag, perm
	OpenFile(ufi string, flag int, perm os.FileMode) (File, error)
	// OpenAvatarFile opens avatar file by ufi
	OpenAvatarFile(ufi string) (File, error)
	// Stat gets file stat info by ufi
	Stat(ufi string) (*sdkdto.SiyouFileInfo, error)
	// MkdirAll creates multi-level directories by ufi
	MkdirAll(ufi string) error
	// Remove removes file by ufi
	Remove(ufi string) error
	// RemoveAll removes dir and any children it contains by ufi
	RemoveAll(ufi string) error
	// Rename renames file
	Rename(oldUFI string, newUFI string) error
	// MoveTrash moves file to trash by ufi
	MoveTrash(ufi string) error
	// Chtimes changes the access and modification times by ufi
	Chtimes(ufi string, atime time.Time, mtime time.Time) error
	// FileExists checks if the file exists by ufi
	FileExists(ufi string) bool
	// FileList get file list
	FileList(options *sdkdto.FileListOptionsV2) *sdkdto.FileListRes
	// Exec execs sql operation
	Exec(f func(*gorm.DB) error) error
	// AppOpenFile opens app file
	AppOpenFile(path string, flag int, perm os.FileMode) (File, error)
	// AppMkdirAll creates multi-level directories in app
	AppMkdirAll(path string) error
	// AppRemoveAll removes dir and any children it contains in app
	AppRemoveAll(path string) error
	// AppFileExists checks if the file exists in app
	AppFileExists(path string) bool
	// AppGenUFI gens the app ufi
	AppGenUFI(path string) string
}

type File interface {
	io.Closer
	io.Reader
	io.ReaderAt
	io.Seeker
	io.Writer
	io.WriterAt

	Name() string
	Readdir(count int) ([]*sdkdto.SiyouFileInfo, error)
	Readdirnames(n int) ([]string, error)
	Stat() (*sdkdto.SiyouFileInfo, error)
	Sync() error
	Truncate(size int64) error
	WriteString(s string) (ret int, err error)
}
