package siyouyunfaas

import (
	"os"
	"path/filepath"
)

// AppOpen 打开app存储空间文件
func (fs *FS) AppOpen(path string) (*os.File, error) {
	return os.OpenFile(filepath.Join(fs.appNormalPath, path), os.O_RDONLY, 0)
}

// AppMkdir 在app存储空间文件创建目录
func (fs *FS) AppMkdir(path string) error {
	return os.MkdirAll(filepath.Join(fs.appNormalPath, path), os.ModePerm)
}

// AppOpenFile 打开或创建app存储空间文件
func (fs *FS) AppOpenFile(path string, flag int, perm os.FileMode) (*os.File, error) {
	return os.OpenFile(filepath.Join(fs.appNormalPath, path), flag, perm)
}

// AppRemove 删除app存储空间文件
func (fs *FS) AppRemove(path string) error {
	return os.Remove(filepath.Join(fs.appNormalPath, path))
}

// AppRename 重命名app存储空间文件
func (fs *FS) AppRename(oldPath, newPath string) error {
	return os.Rename(filepath.Join(fs.appNormalPath, oldPath), filepath.Join(fs.appNormalPath, newPath))
}
