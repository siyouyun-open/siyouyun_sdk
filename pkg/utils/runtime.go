package utils

import "os"

// IsRUNC returns true when running in RUNC container mode (APP_RUNTIME=RUNC).
func IsRUNC() bool {
	return os.Getenv("APP_RUNTIME") == "RUNC"
}
