package sdkdto

type FileInfoRes struct {
	Id          uint64 `json:"id"`          // 文件inode
	Name        string `json:"name"`        // 文件名称
	IsDir       bool   `json:"isDir"`       // 是否是文件夹
	ParentInode uint64 `json:"parentInode"` // 父级inode
	ParentPath  string `json:"parentPath"`  // 父级路径
	FullPath    string `json:"fullPath"`    // 全路径
	Mime        string `json:"mime"`        // 文件媒体类型
	Atime       int64  `json:"atime"`       // access time
	Mtime       int64  `json:"mtime"`       // modify time
	Ctime       int64  `json:"ctime"`       // change time
	Size        int64  `json:"size"`        // 文件大小
	HasAvatar   bool   `json:"hasAvatar"`   // 是否有替身
}
