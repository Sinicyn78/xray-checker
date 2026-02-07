package logger

import (
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

type Level int

const (
	LevelNone Level = iota
	LevelError
	LevelWarn
	LevelInfo
	LevelDebug
)

var (
	level       = LevelInfo
	errorLogger = log.New(os.Stderr, "", log.LstdFlags)
	stdLogger   = log.New(os.Stdout, "", log.LstdFlags)
	logFile     *os.File
	mu          sync.Mutex
)

func ParseLevel(s string) Level {
	switch strings.ToLower(s) {
	case "none", "off", "silent":
		return LevelNone
	case "error", "err":
		return LevelError
	case "warn", "warning":
		return LevelWarn
	case "info":
		return LevelInfo
	case "debug":
		return LevelDebug
	default:
		return LevelInfo
	}
}

func (l Level) String() string {
	switch l {
	case LevelNone:
		return "none"
	case LevelError:
		return "error"
	case LevelWarn:
		return "warn"
	case LevelInfo:
		return "info"
	case LevelDebug:
		return "debug"
	default:
		return "unknown"
	}
}

func SetLevel(l Level) {
	mu.Lock()
	defer mu.Unlock()
	level = l
	applyOutputsLocked()
}

func SetFile(path string) error {
	mu.Lock()
	defer mu.Unlock()

	if logFile != nil {
		_ = logFile.Close()
		logFile = nil
	}
	if strings.TrimSpace(path) == "" {
		applyOutputsLocked()
		return nil
	}

	clean := filepath.Clean(path)
	if dir := filepath.Dir(clean); dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}

	f, err := os.OpenFile(clean, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	logFile = f
	applyOutputsLocked()
	return nil
}

func applyOutputsLocked() {
	if level == LevelNone {
		stdLogger.SetOutput(io.Discard)
		errorLogger.SetOutput(io.Discard)
		return
	}

	stdOut := io.Writer(os.Stdout)
	errOut := io.Writer(os.Stderr)
	if logFile != nil {
		stdOut = io.MultiWriter(os.Stdout, logFile)
		errOut = io.MultiWriter(os.Stderr, logFile)
	}
	stdLogger.SetOutput(stdOut)
	errorLogger.SetOutput(errOut)
}

func Debug(format string, v ...interface{}) {
	if level >= LevelDebug {
		stdLogger.Printf("[DEBUG] "+format, v...)
	}
}

func Info(format string, v ...interface{}) {
	if level >= LevelInfo {
		stdLogger.Printf(format, v...)
	}
}

func Warn(format string, v ...interface{}) {
	if level >= LevelWarn {
		stdLogger.Printf("[WARN] "+format, v...)
	}
}

func Error(format string, v ...interface{}) {
	if level >= LevelError {
		errorLogger.Printf("[ERROR] "+format, v...)
	}
}

func Fatal(format string, v ...interface{}) {
	errorLogger.Fatalf("[FATAL] "+format, v...)
}

func Startup(format string, v ...interface{}) {
	if level >= LevelInfo {
		stdLogger.Printf(format, v...)
		return
	}
	fmt.Printf(format+"\n", v...)
}

func Result(format string, v ...interface{}) {
	if level >= LevelInfo {
		stdLogger.Printf(format, v...)
	}
}
