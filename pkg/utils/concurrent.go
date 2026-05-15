package utils

import (
	"runtime/debug"

	sdklog "github.com/siyouyun-open/siyouyun_sdk/pkg/log"
	"golang.org/x/sync/errgroup"
)

// SafeGo 在 goroutine 中执行任务并自动 recover
func SafeGo(task func()) {
	go func() {
		defer func() {
			if r := recover(); r != nil {
				sdklog.Logger.Errorf("Recovered from panic: %v\nStack trace:\n%s", r, debug.Stack())
			}
		}()
		task()
	}()
}

// SafeGoWithErrGroup 在 errgroup 中执行任务并自动 recover
func SafeGoWithErrGroup(eg *errgroup.Group, task func() error) {
	eg.Go(func() error {
		defer func() {
			if r := recover(); r != nil {
				sdklog.Logger.Errorf("Recovered from panic: %v\nStack trace:\n%s", r, debug.Stack())
			}
		}()
		return task()
	})
}
