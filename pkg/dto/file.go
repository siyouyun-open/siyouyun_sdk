package sdkdto

type FileInfoRes struct {
	Id          int64  `json:"id"`
	Inode       uint64 `json:"inode"`
	Name        string `json:"name"`
	Size        int64  `json:"size"`
	ParentInode uint64 `json:"parentInode"`
	ParentPath  string `json:"parentPath"`
	FullPath    string `json:"fullPath"`
	Md51        string `json:"md51"`
	Md52        string `json:"md52"`
	Md53        string `json:"md53"`
	Mime        string `json:"mime"`
	Owner       string `json:"owner"`
	Atime       int64  `json:"atime"`
	Mtime       int64  `json:"mtime"`
	Ctime       int64  `json:"ctime"`
	// 是否有替身
	HasAvatar bool `json:"hasAvatar"`
	// 是否是文件夹
	IsDir bool `json:"isDir"`
}
