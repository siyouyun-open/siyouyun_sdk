package siyouyunfaas

import (
	"github.com/siyouyun-open/siyouyun_sdk/utils"
	"gorm.io/gorm"
	"path/filepath"
	"strconv"
	"strings"
)

// PathToInode fullpath为用户空间的相对fullpath
func PathToInode(app *App, un *utils.UserNamespace, fullpath string) int64 {
	if fullpath == "/" {
		return 1
	}
	paths := strings.Split(fullpath, "/")
	var resInode int64 = 0
	for _, path := range paths {
		strings.ReplaceAll(strings.TrimSpace(path), "/", "")
		var thisEdge Edge
		err := app.execByUn(un, func(db *gorm.DB) error {
			return db.Model(&Edge{Name: path, Parent: resInode}).First(&thisEdge).Error
		})
		if err != nil {
			return -1
		}
		resInode = thisEdge.Inode
	}
	return resInode
}

// InodeToPath inode转fullpath
func InodeToPath(app *App, un *utils.UserNamespace, inode int64) string {
	var edge Edge
	err := app.execByUn(un, func(db *gorm.DB) error {
		return db.Where("inode = ?", inode).First(&edge).Error
	})
	if err != nil {
		return ""
	}
	inodes := strings.Split(edge.FullPath, "-")
	var resPath = "/"
	for _, subInode := range inodes {
		var thisEdge Edge
		ino, _ := strconv.ParseInt(subInode, 10, 64)
		if ino == 1 {
			continue
		}
		err = app.execByUn(un, func(db *gorm.DB) error {
			return db.Where("inode = ?", ino).First(&thisEdge).Error
		})
		if err != nil {
			return ""
		}
		resPath = filepath.Join(resPath, thisEdge.Name)
	}
	return resPath
}

func (a *App) execByUn(un *utils.UserNamespace, f func(*gorm.DB) error) error {
	err := a.db.Transaction(func(tx *gorm.DB) (err error) {
		dbname := un.DatabaseName()
		if dbname == "" {
			return
		}
		err = tx.Exec("use " + dbname).Error
		if err != nil {
			return err
		}
		err = f(tx)
		if err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return err
	}
	return nil
}
