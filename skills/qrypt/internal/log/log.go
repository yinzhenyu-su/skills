package log

import (
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"time"

	"gopkg.in/natefinch/lumberjack.v2"
)

type Level int

const (
	LevelDebug Level = iota
	LevelInfo
	LevelWarn
	LevelError
	LevelOff
)

func ParseLevel(s string) Level {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "debug":
		return LevelDebug
	case "info", "":
		return LevelInfo
	case "warn", "warning":
		return LevelWarn
	case "error":
		return LevelError
	case "off", "none":
		return LevelOff
	default:
		return LevelInfo
	}
}

func (l Level) String() string {
	switch l {
	case LevelDebug:
		return "DEBUG"
	case LevelInfo:
		return "INFO"
	case LevelWarn:
		return "WARN"
	case LevelError:
		return "ERROR"
	case LevelOff:
		return "OFF"
	default:
		return "UNKNOWN"
	}
}

type RotateConfig struct {
	MaxSize    int
	MaxBackups int
	MaxAge     int
	Compress   bool
}

var DefaultRotateConfig = RotateConfig{
	MaxSize:    100,
	MaxBackups: 7,
	MaxAge:     28,
	Compress:   true,
}

type Logger struct {
	level  Level
	writer io.Writer
	lj     *lumberjack.Logger
	mu     sync.Mutex
}

func New(level string, logFile string, rotate *RotateConfig) (*Logger, error) {
	l := &Logger{level: ParseLevel(level)}

	if logFile != "" {
		rc := DefaultRotateConfig
		if rotate != nil {
			if rotate.MaxSize > 0 {
				rc.MaxSize = rotate.MaxSize
			}
			if rotate.MaxBackups > 0 {
				rc.MaxBackups = rotate.MaxBackups
			}
			if rotate.MaxAge > 0 {
				rc.MaxAge = rotate.MaxAge
			}
			rc.Compress = rotate.Compress
		}
		lj := &lumberjack.Logger{
			Filename:   logFile,
			MaxSize:    rc.MaxSize,
			MaxBackups: rc.MaxBackups,
			MaxAge:     rc.MaxAge,
			Compress:   rc.Compress,
		}
		l.writer = lj
		l.lj = lj
	} else {
		l.writer = os.Stdout
	}

	return l, nil
}

func (l *Logger) logf(level Level, format string, v ...interface{}) {
	if level < l.level {
		return
	}
	msg := fmt.Sprintf(format, v...)
	ts := time.Now().Format("2006-01-02 15:04:05")
	line := fmt.Sprintf("[%s] %s %s", ts, level.String(), msg)

	l.mu.Lock()
	fmt.Fprint(l.writer, line)
	l.mu.Unlock()
}

func (l *Logger) Debugf(format string, v ...interface{})   { l.logf(LevelDebug, format, v...) }
func (l *Logger) Infof(format string, v ...interface{})    { l.logf(LevelInfo, format, v...) }
func (l *Logger) Warnf(format string, v ...interface{})    { l.logf(LevelWarn, format, v...) }
func (l *Logger) Errorf(format string, v ...interface{})   { l.logf(LevelError, format, v...) }

func (l *Logger) Debug(args ...interface{})   { l.logf(LevelDebug, "%s", fmt.Sprint(args...)) }
func (l *Logger) Info(args ...interface{})    { l.logf(LevelInfo, "%s", fmt.Sprint(args...)) }
func (l *Logger) Warn(args ...interface{})    { l.logf(LevelWarn, "%s", fmt.Sprint(args...)) }
func (l *Logger) Error(args ...interface{})   { l.logf(LevelError, "%s", fmt.Sprint(args...)) }

func (l *Logger) Rotate() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.lj != nil {
		return l.lj.Rotate()
	}
	return nil
}

func (l *Logger) Close() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.lj != nil {
		return l.lj.Close()
	}
	if c, ok := l.writer.(io.Closer); ok {
		return c.Close()
	}
	return nil
}

var L = NewDefault()

func NewDefault() *Logger {
	l, err := New("info", "", nil)
	if err != nil {
		panic(err)
	}
	return l
}
