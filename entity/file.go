package entity

// Edge 文件树
type Edge struct {
	Parent         int64
	Name           string
	Inode          int64
	PosixType      int
	MimeGroup      string
	MimeDetail     string
	FullPath       string
	FullParentPath string
}

type FileInfo struct {
	Inode         int64
	MD5           string
	OwnerUsername string
	Name          string
	PosixType     int
	Mode          int
	Atime         int64
	Mtime         int64
	Ctime         int64
	Length        int64
	MimeGroup     string
	MimeDetail    string
	FullPath      string
	Score         string
}
