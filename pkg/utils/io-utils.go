package utils

import "sync"

// ReadBufPool is the pool used for read buffers.
var ReadBufPool = sync.Pool{
	New: func() interface{} {
		b := make([]byte, 1024)
		return &b
	},
}
