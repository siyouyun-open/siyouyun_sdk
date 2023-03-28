package sdkdto

type FileInfoRes struct {
	Id         int64  `json:"id"`
	Inode      int64  `json:"inode"`
	Name       string `json:"name"`
	Size       int64  `json:"size"`
	ParentPath string `json:"parentPath"`
	FullPath   string `json:"fullPath"`
	PosixType  int    `json:"-"`
	Md51       string `json:"md51"`
	Md52       string `json:"md52"`
	Md53       string `json:"md53"`
	Mime       string `json:"mime"`
	Owner      string `json:"owner"`
	Atime      int64  `json:"atime"`
	Mtime      int64  `json:"mtime"`
	Ctime      int64  `json:"ctime"`

	// 下述需要计算的属性
	HasThumbnail bool `json:"hasThumbnail"`
	// 扩展名
	Extension string `json:"extension"`
	// 是否是文件夹
	IsDir bool `json:"isDir"`
	// 文件详情的事件列表
	EventList interface{} `json:"eventList,omitempty"`
	// 分享文件的下载次数
	ShareDownloads int `json:"shareDownloads,omitempty"`
}
