package siyouyunsdk

import (
	"io"
	"log"
	"net"
	"os"
	"syscall"
	"time"
)

// SyyFile 四有云File，FaaS应用操作文件均使用此文件结构体，覆盖了os.File的所有方法实现
type SyyFile struct {
	file           *os.File
	unixConn       *net.UnixConn
	unixSocketPath string
}

func (sf *SyyFile) Close() error {
	log.Printf("[DEBUG] close file, socket path: %s", sf.unixSocketPath)
	if sf.file != nil {
		_ = sf.file.Close()
	}
	if sf.unixConn != nil {
		_ = sf.unixConn.Close()
	}
	if sf.unixSocketPath == "" {
		return nil
	}
	return os.Remove(sf.unixSocketPath)
}

func (sf *SyyFile) Chmod(mode os.FileMode) error {
	return sf.file.Chmod(mode)
}

func (sf *SyyFile) Chdir() error {
	return sf.file.Chdir()
}

func (sf *SyyFile) Chown(uid, gid int) error {
	return sf.file.Chown(uid, gid)
}

func (sf *SyyFile) Fd() uintptr {
	return sf.file.Fd()
}

func (sf *SyyFile) Name() string {
	return sf.file.Name()
}

func (sf *SyyFile) Read(b []byte) (n int, err error) {
	return sf.file.Read(b)
}

func (sf *SyyFile) ReadAt(b []byte, off int64) (n int, err error) {
	return sf.file.ReadAt(b, off)
}

func (sf *SyyFile) ReadDir(n int) ([]os.DirEntry, error) {
	return sf.file.ReadDir(n)
}

func (sf *SyyFile) Readdir(n int) ([]os.FileInfo, error) {
	return sf.file.Readdir(n)
}

func (sf *SyyFile) Readdirnames(n int) (names []string, err error) {
	return sf.file.Readdirnames(n)
}

func (sf *SyyFile) ReadFrom(r io.Reader) (n int64, err error) {
	return sf.file.ReadFrom(r)
}

func (sf *SyyFile) Stat() (os.FileInfo, error) {
	return sf.file.Stat()
}

func (sf *SyyFile) Seek(offset int64, whence int) (ret int64, err error) {
	return sf.file.Seek(offset, whence)
}

func (sf *SyyFile) Sync() error {
	return sf.file.Sync()
}

func (sf *SyyFile) SetWriteDeadline(t time.Time) error {
	return sf.file.SetWriteDeadline(t)
}

func (sf *SyyFile) SetReadDeadline(t time.Time) error {
	return sf.file.SetReadDeadline(t)
}

func (sf *SyyFile) SetDeadline(t time.Time) error {
	return sf.file.SetDeadline(t)
}

func (sf *SyyFile) SyscallConn() (syscall.RawConn, error) {
	return sf.file.SyscallConn()
}

func (sf *SyyFile) Truncate(size int64) error {
	return sf.file.Truncate(size)
}

func (sf *SyyFile) Write(b []byte) (n int, err error) {
	return sf.file.Write(b)
}

func (sf *SyyFile) WriteAt(b []byte, off int64) (n int, err error) {
	return sf.file.WriteAt(b, off)
}

func (sf *SyyFile) WriteString(s string) (n int, err error) {
	return sf.file.WriteString(s)
}
