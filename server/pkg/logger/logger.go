package logger

import (
	"fmt"
	"time"
)

// ANSI Color Codes
const (
	Reset  = "\033[0m"
	Red    = "\033[31m"
	Green  = "\033[32m"
	Yellow = "\033[33m"
	Blue   = "\033[34m"
	Purple = "\033[35m"
	Cyan   = "\033[36m"
	Gray   = "\033[37m"
)

func log(level, color, module, msg string, args ...interface{}) {
	timestamp := time.Now().Format("15:04:05")
	formattedMsg := fmt.Sprintf(msg, args...)
	fmt.Printf("%s[%s] [%s] %s: %s%s\n", color, timestamp, level, module, formattedMsg, Reset)
}

func Info(module, msg string, args ...interface{}) {
	log("INFO", Green, module, msg, args...)
}

func Warn(module, msg string, args ...interface{}) {
	log("WARN", Yellow, module, msg, args...)
}

func Error(module, msg string, args ...interface{}) {
	log("ERROR", Red, module, msg, args...)
}

func Debug(module, msg string, args ...interface{}) {
	log("DEBUG", Cyan, module, msg, args...)
}

func Fatal(module, msg string, args ...interface{}) {
	log("FATAL", Red, module, msg, args...)
	// Typically fatal might panic or exit, but for a web server helper verify if we want to exit.
	// Standard log.Fatal exits. Here keeping it simple as just logging.
	// If exit is needed, caller can do os.Exit(1)
}
