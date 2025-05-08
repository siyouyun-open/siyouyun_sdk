package sdklog

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"runtime"

	"github.com/sirupsen/logrus"
)

var Logger = logrus.New()

// InitLogger initializes the logger with default settings.
func InitLogger(level logrus.Level) {
	log.SetFlags(0)
	log.SetOutput(Logger.Writer())
	// Default to standard output
	Logger.SetOutput(os.Stdout)
	// Set log level
	Logger.SetLevel(level)
	// Set report caller
	Logger.SetReportCaller(true)
	// Set log format
	Logger.SetFormatter(&logrus.TextFormatter{
		DisableTimestamp: true,
		CallerPrettyfier: func(frame *runtime.Frame) (function string, file string) {
			fileName := filepath.Base(frame.File)
			return "", fmt.Sprintf("%s:%d", fileName, frame.Line)
		},
	})
}
