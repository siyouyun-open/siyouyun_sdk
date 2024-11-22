package sdklog

import (
	"github.com/sirupsen/logrus"
	"log"
	"os"
	"path/filepath"
)

var Logger = logrus.New()

type SimpleFormatter struct {
	logrus.TextFormatter
}

func (f *SimpleFormatter) Format(entry *logrus.Entry) ([]byte, error) {
	// Trim the caller to only include the file name
	if entry.HasCaller() {
		file := filepath.Base(entry.Caller.File)
		entry.Caller.File = file
	}
	return f.TextFormatter.Format(entry)
}

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
	Logger.SetFormatter(&SimpleFormatter{
		logrus.TextFormatter{
			DisableTimestamp: true,
		},
	})
}
