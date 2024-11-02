package sdkdto

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
