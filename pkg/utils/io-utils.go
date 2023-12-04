package utils

import "sync"

// ReadBufPool 读取缓冲池
var ReadBufPool = sync.Pool{
	New: func() interface{} {
		b := make([]byte, 1024)
		return &b
	},
}
