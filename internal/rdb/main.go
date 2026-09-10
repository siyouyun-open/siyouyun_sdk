package rdb

import (
	"log"
	"os"
	"time"

	"gorm.io/gorm/logger"
)

func NewLogger() logger.Interface {
	return logger.New(
		log.New(os.Stdout, "\r\n", log.LstdFlags), // io writer (log output target)
		logger.Config{
			SlowThreshold:             time.Second,  // slow SQL threshold
			LogLevel:                  logger.Error, // log level
			IgnoreRecordNotFoundError: true,         // ignore ErrRecordNotFound errors
			Colorful:                  false,        // disable coloured output
		},
	)
}
