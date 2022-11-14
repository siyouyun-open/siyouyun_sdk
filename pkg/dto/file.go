package sdkdto

type FileInfoRes struct {
	Id           int64       `json:"id"`
	HasThumbnail bool        `json:"hasThumbnail"`
	Name         string      `json:"name"`
	Size         int64       `json:"size"`
	ParentPath   string      `json:"parentPath"`
	FullPath     string      `json:"fullPath"`
	IsDir        bool        `json:"isDir"`
	Tag          string      `json:"tag"`
	Md51         string      `json:"md51"`
	Md52         string      `json:"md52"`
	Md53         string      `json:"md53"`
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
