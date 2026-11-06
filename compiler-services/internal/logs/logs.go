package logs

import (
	"log"
	"os"
)

var logger = log.New(os.Stderr, "", log.LstdFlags|log.LUTC)

func Info(format string, args ...any) {
	logger.Printf("INFO "+format, args...)
}

func Error(format string, args ...any) {
	logger.Printf("ERROR "+format, args...)
}

func Fatal(format string, args ...any) {
	logger.Fatalf("FATAL "+format, args...)
}
