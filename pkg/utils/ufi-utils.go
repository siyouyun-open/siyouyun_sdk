package utils

import (
	"fmt"
	"path/filepath"
	"strings"
)

type UFIProtocol string

const (
	UFIv1 UFIProtocol = "ufi"
)

func (U UFIProtocol) String() string {
	return string(U)
}

// UFIMode ufi文件定位模式
type UFIMode string

func (U UFIMode) String() string {
	return string(U)
}

const (
	IdUFI   UFIMode = "id"
	PathUFI UFIMode = "path"
)

type StorageType string

const (
	RawDisk     StorageType = "raw-disk"
	USB         StorageType = "usb"
	Baiduyun    StorageType = "baiduyun"
	CommonOSS   StorageType = "oss"
	AliyunOSS   StorageType = "aliyun-oss"
	TencentOSS  StorageType = "tencent-oss"
	SambaClient StorageType = "smb-client"
	UFSMeta     StorageType = "siyouyun-meta"
	UFSRaw      StorageType = "siyouyun-raw"
	UFSSandbox  StorageType = "siyouyun-sandbox"
	Snapshot    StorageType = "siyouyun-snapshot"
	Trash       StorageType = "siyouyun-trash"
	Webdav      StorageType = "webdav"
	FTP         StorageType = "ftp"
)

func (t StorageType) String() string {
	return string(t)
}

func (t StorageType) IsSiyouyunStorage() bool {
	return strings.HasPrefix(string(t), "siyouyun")
}

// IdentifierType 定义identifier类型约束接口
type IdentifierType interface {
	~int | ~int32 | ~int64 | ~uint | ~uint32 | uint64 | ~string
}

// UFI 统一文件定位
type UFI struct {
	ufiProtocol UFIProtocol
	StorageType StorageType `json:"storageType"`
	UUID        string      `json:"uuid"`
	Mode        UFIMode     `json:"mode"`       // 标识符类型: id path
	Identifier  string      `json:"identifier"` // 标识符正文
}

func (ufi *UFI) Validate() bool {
	switch ufi.ufiProtocol {
	case UFIv1:
	default:
		return false
	}
	switch ufi.Mode {
	case IdUFI:
	case PathUFI:
	default:
		return false
	}
	return true
}

// Serialize
// siyouyun-pool的uuid需要同时传递UGN确定存储空间
// usb的uuid是label的hash,不包含UGN信息,由权限管理控制访问
// 其余存储的uuid隐藏含义包含了UGN信息
// eg:
//   - 样板
//     /ufi/{storageType}/{uuid}/id/110
//     /ufi/{storageType}/{uuid}/path/123.png
func (ufi *UFI) Serialize() string {
	return filepath.Join(
		"/",
		UFIv1.String(),
		ufi.StorageType.String(),
		ufi.UUID,
		ufi.Mode.String(),
		strings.TrimRight(fmt.Sprintf("%v", ufi.Identifier), "/"),
	)
}

func NewUFI[T IdentifierType](storageType StorageType, uuid string, mode UFIMode, identifier T) *UFI {
	return &UFI{
		ufiProtocol: UFIv1,
		StorageType: storageType,
		UUID:        uuid,
		Mode:        mode,
		Identifier:  fmt.Sprintf("%v", identifier),
	}
}
