package sdkdto

import (
	sdkconst "github.com/siyouyun-open/siyouyun_sdk/pkg/const"
	"github.com/siyouyun-open/siyouyun_sdk/pkg/utils"
)

// SiyouFileInfo siyouyun file info
type SiyouFileInfo struct {
	Id        uint64 `json:"id"`              // file id
	Name      string `json:"name"`            // file name
	IsDir     bool   `json:"isDir"`           // if it's dir
	UFI       string `json:"ufi"`             // file ufi
	PUFI      string `json:"pufi"`            // file parent ufi
	Mime      string `json:"mime"`            // file mime type
	Atime     int64  `json:"atime"`           // access time
	Mtime     int64  `json:"mtime"`           // modify time
	Ctime     int64  `json:"ctime"`           // change time
	Size      int64  `json:"size"`            // file size
	HasAvatar bool   `json:"hasAvatar"`       // if has avatar
	Hash1     string `json:"hash1,omitempty"` // hash level1
	Hash2     string `json:"hash2,omitempty"` // hash level2
	Hash3     string `json:"hash3,omitempty"` // hash level3
	Owner     string `json:"owner,omitempty"` // owner
}

// FileListOptionsV2 file list options
type FileListOptionsV2 struct {
	UGN        utils.UserGroupNamespace `json:"ugn"`
	PUFI       string                   `json:"pufi"`
	Page       int                      `json:"page"`
	PageSize   int                      `json:"pageSize"`
	NextMarker string                   `json:"nextMarker"`
	ShowHidden bool                     `json:"showHidden"`
	MimeGroup  string                   `json:"mimeGroup"`
	MimeDetail string                   `json:"mimeDetail"`
	OrderBy    string                   `json:"orderBy"`
	OrderASC   bool                     `json:"orderASC"`
	SearchAll  bool                     `json:"searchAll"`
	NameSearch string                   `json:"nameSearch"`
	MediaType  sdkconst.MediaType       `json:"mediaType"`
	Exts       []string                 `json:"exts"`
	Recursion  bool                     `json:"recursion"`
	OnlyDir    bool                     `json:"onlyDir"`
	OnlyFile   bool                     `json:"onlyFile"`
	Streaming  bool                     `json:"streaming"`
	Skip       int                      `json:"skip"`
	Limit      int                      `json:"limit"`
}

// FileListRes file list result
type FileListRes struct {
	Page       int              `json:"page"`
	PageSize   int              `json:"pageSize"`
	Total      int64            `json:"total"`
	HasMore    bool             `json:"hasMore"`
	NextMarker string           `json:"nextMarker"`
	Data       []*SiyouFileInfo `json:"data"`
}
