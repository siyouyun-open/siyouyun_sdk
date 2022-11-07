package siyouyunfaas

import (
	"errors"
	"fmt"
	"github.com/kataras/iris/v12"
	sdkconst "github.com/siyouyun-open/siyouyun_sdk/const"
	"github.com/siyouyun-open/siyouyun_sdk/entity"
	"github.com/siyouyun-open/siyouyun_sdk/utils"
	"gorm.io/gorm"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// FS 事件fs
type FS struct {
	*utils.UserNamespace
	EventFileInode int64
	AppCodeName    string

	app           *App
	mntPath       string
	appNormalPath string
}

type FileInfoRes struct {
	Id           int64       `json:"id"`
	HasThumbnail bool        `json:"hasThumbnail"`
	Name         string      `json:"name"`
	Size         int64       `json:"size"`
	ParentPath   string      `json:"parentPath"`
	FullPath     string      `json:"fullPath"`
	IsDir        bool        `json:"isDir"`
	Tag          string      `json:"tag"`
	Md5          string      `json:"md5"`
	Extension    string      `json:"extension"`
	Mime         string      `json:"mime"`
	Owner        string      `json:"owner"`
	Atime        int64       `json:"atime"`
	Mtime        int64       `json:"mtime"`
	Ctime        int64       `json:"ctime"`
	Tags         interface{} `json:"tags"`
	Ext0         interface{} `json:"ext0"`
	Ext1         interface{} `json:"ext1"`
	Ext2         interface{} `json:"ext2"`

	EventList interface{} `json:"eventList,omitempty"`
}

func (a *App) NewFSFromCtx(ctx iris.Context) *FS {
	un := utils.NewUserNamespaceFromIris(ctx)
	fs := &FS{
		UserNamespace: un,
		AppCodeName:   a.AppCode,
		app:           a,
	}
	fs.mntPath = fs.getMntPrefix()
	fs.appNormalPath = fs.getNormalAppPrefix()
	return fs
}

func (a *App) newEventFSFromFileEvent(fe *FileEvent) *FS {
	un := &utils.UserNamespace{
		Username:  fe.Username,
		Namespace: fe.Namespace,
	}
	efs := &FS{
		UserNamespace:  un,
		EventFileInode: fe.Inode,
		AppCodeName:    a.AppCode,
		app:            a,
	}
	efs.mntPath = efs.getMntPrefix()
	efs.appNormalPath = efs.getNormalAppPrefix()
	return efs
}

func (a *App) exec(un *utils.UserNamespace, f func(*gorm.DB) error) error {
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

func (fs *FS) getMntPrefix() string {
	var prefixPath string
	switch fs.Namespace {
	case "":
		fallthrough
	case sdkconst.MainNamespace:
		prefixPath = filepath.Join(
			sdkconst.FaasMntPrefix,
			fs.Username,
			strings.Join([]string{fs.Username, sdkconst.MainNamespace}, "-"),
		)
	case sdkconst.PrivateNamespace:
		prefixPath = filepath.Join(
			sdkconst.FaasMntPrefix,
			fs.Username,
			strings.Join([]string{fs.Username, sdkconst.PrivateNamespace}, "-"),
		)
	case sdkconst.CommonNamespace:
		prefixPath = filepath.Join(sdkconst.FaasMntPrefix, sdkconst.CommonNamespace)
	}
	return prefixPath
}

func (fs *FS) getNormalAppPrefix() string {
	var prefixPath string
	switch fs.Namespace {
	case "":
		fallthrough
	case sdkconst.MainNamespace:
		prefixPath = filepath.Join(
			sdkconst.FaasMntPrefix,
			fs.Username,
			strings.Join([]string{fs.Username, sdkconst.MainNamespace}, "-"),
			".siyouyun",
			fs.AppCodeName,
		)
	case sdkconst.PrivateNamespace:
		prefixPath = filepath.Join(
			sdkconst.FaasMntPrefix,
			fs.Username,
			strings.Join([]string{fs.Username, sdkconst.PrivateNamespace}, "-"),
			".siyouyun",
			fs.AppCodeName,
		)
	case sdkconst.CommonNamespace:
		prefixPath = filepath.Join(
			sdkconst.FaasMntPrefix,
			sdkconst.CommonNamespace,
			".siyouyun",
			fs.AppCodeName,
		)
	}
	return prefixPath
}

const (
	fileInfoColumn = `e.inode, IFNULL(fi.md5,"") as md5, IFNULL(fi.owner_username,"") as owner_username, name, e.type, mode, atime, 
	mtime, ctime, length,mime_group, mime_detail, full_path`
)

// PathToInode fullpath为用户空间的相对fullpath
func (fs *FS) PathToInode(fullpath string) int64 {
	if fullpath == "/" {
		return 1
	}
	paths := strings.Split(fullpath, "/")
	var resInode int64 = 0
	for _, path := range paths {
		strings.ReplaceAll(strings.TrimSpace(path), "/", "")
		var thisEdge entity.Edge
		err := fs.app.exec(fs.UserNamespace, func(db *gorm.DB) error {
			return db.Model(&entity.Edge{Name: path, Parent: resInode}).First(&thisEdge).Error
		})
		if err != nil {
			return -1
		}
		resInode = thisEdge.Inode
	}
	return resInode
}

// InodeToPath inode转fullpath
func (fs *FS) InodeToPath(inode int64) string {
	var edge entity.Edge
	err := fs.app.exec(fs.UserNamespace, func(db *gorm.DB) error {
		return db.Where("inode = ?", inode).First(&edge).Error
	})
	if err != nil {
		return ""
	}
	inodes := strings.Split(edge.FullPath, "-")
	var resPath = "/"
	for _, subInode := range inodes {
		var thisEdge entity.Edge
		ino, _ := strconv.ParseInt(subInode, 10, 64)
		if ino == 1 {
			continue
		}
		err = fs.app.exec(fs.UserNamespace, func(db *gorm.DB) error {
			return db.Where("inode = ?", ino).First(&thisEdge).Error
		})
		if err != nil {
			return ""
		}
		resPath = filepath.Join(resPath, thisEdge.Name)
	}
	return resPath
}

func (fs *FS) InodeToFileInfo(inode string) *FileInfoRes {
	selectSql := fmt.Sprintf(`
		SELECT %v FROM jfs_edge AS e 
        LEFT JOIN siyou_basic_fileinfo AS fi ON fi.inode = e.inode 
        LEFT JOIN jfs_node AS n ON n.inode = e.inode 
        WHERE e.inode = %v`,
		fileInfoColumn, inode)
	var res FileInfoRes
	var fi entity.FileInfo
	err := fs.app.exec(fs.UserNamespace, func(db *gorm.DB) error {
		rows, err := db.Raw(selectSql).Rows()
		if err != nil {
			return nil
		}
		for rows.Next() {
			db.ScanRows(rows, &fi)
			return nil
		}
		return nil
	})
	if err != nil {
		return nil
	}
	fi.FullPath = fs.InodeToPath(fi.Inode)
	return &res
}

func (fs *FS) InodesToFileInfos(inodes ...string) []FileInfoRes {
	selectSql := fmt.Sprintf(`
		SELECT %v FROM jfs_edge AS e 
        LEFT JOIN siyou_basic_fileinfo AS fi ON fi.inode = e.inode 
        LEFT JOIN jfs_node AS n ON n.inode = e.inode 
        WHERE e.inode in (%v)`,
		fileInfoColumn, strings.Join(inodes, ","))
	var res []FileInfoRes
	var fis []entity.FileInfo
	err := fs.app.exec(fs.UserNamespace, func(db *gorm.DB) error {
		rows, err := db.Raw(selectSql).Rows()
		if err != nil {
			return nil
		}
		for rows.Next() {
			var fi entity.FileInfo
			db.ScanRows(rows, &fi)
			fis = append(fis, fi)
		}
		return nil
	})
	if err != nil {
		return nil
	}
	for i := range fis {
		fis[i].FullPath = fs.InodeToPath(fis[i].Inode)
	}
	return res
}

func (fs *FS) fileInfosToFileRes(fileInfos []entity.FileInfo) []FileInfoRes {
	var res []FileInfoRes
	for _, fi := range fileInfos {
		if fi.Name == "" {
			continue
		}
		ext := filepath.Ext(fi.Name)
		var m string
		if ext == "" {
			m = ""
		} else {
			m = strings.Join([]string{fi.MimeGroup, fi.MimeDetail}, "/")
		}
		var ap = FileInfoRes{
			Id:           fi.Inode,
			Name:         fi.Name,
			Size:         fi.Length,
			ParentPath:   filepath.Dir(fi.FullPath),
			FullPath:     fi.FullPath,
			IsDir:        fi.PosixType == 2,
			Extension:    ext,
			Mime:         m,
			Md5:          fi.MD5,
			Owner:        fi.OwnerUsername,
			Atime:        fi.Atime,
			Mtime:        fi.Mtime,
			Ctime:        fi.Ctime,
			HasThumbnail: fs.checkThumbnail(&fi),
		}
		// todo add tag info
		// ap.Tags = tags[fi.Id]
		res = append(res, ap)
	}
	return res
}

const (
	ThumbnailDirname = "/.siyouyun/.thumbnail"

	// ThumbnailGeneratingSuffix 正在生成中的suffix
	ThumbnailGeneratingSuffix = ".generating"
	ThumbnailOkaySuffix       = ".done"
)

func (fs *FS) checkThumbnail(fi *entity.FileInfo) bool {
	if FileType(fi.MimeGroup) == FileTypeImage || FileType(fi.MimeGroup) == FileTypeVideo {
		tbPath := filepath.Join(
			ThumbnailDirname,
			strconv.FormatInt(fi.Inode, 10)+ThumbnailOkaySuffix,
		)
		return fs.FileExists(tbPath)
	}
	return false
}

func (fs *FS) EnsureDirExist(ps ...string) {
	for _, p := range ps {
		err := os.MkdirAll(p, os.ModePerm)
		if err != nil && !errors.Is(err, os.ErrExist) {
			return
		}
	}
}

// FileExists 文件是否存在
func (fs *FS) FileExists(path string) bool {
	var prefixPath = fs.mntPath
	stat, err := os.Stat(filepath.Join(prefixPath, path))
	if err != nil && os.IsNotExist(err) {
		return false
	}
	if stat.IsDir() {
		return false
	}
	return true
}

// Open 只读权限打开用户空间文件
func (fs *FS) Open(path string) (*os.File, error) {
	return os.OpenFile(filepath.Join(fs.mntPath, path), os.O_RDONLY, 0)
}
