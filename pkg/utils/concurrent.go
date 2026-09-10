package utils

import (
	"runtime/debug"

	sdklog "github.com/siyouyun-open/siyouyun_sdk/pkg/log"
	"golang.org/x/sync/errgroup"
)

// SafeGo runs the task in a goroutine and recovers from panics automatically.
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

// SafeGoWithErrGroup runs the task in an errgroup and recovers from panics automatically.
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
