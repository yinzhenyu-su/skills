package driver

import (
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"time"

	"gopkg.in/natefinch/lumberjack.v2"
)

// LogLevel defines log severity
type LogLevel int

const (
	LogLevelDebug LogLevel = iota
	LogLevelInfo
	LogLevelWarn
	LogLevelError
	LogLevelOff
)

func parseLogLevel(s string) LogLevel {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "debug":
		return LogLevelDebug
	case "info", "":
		return LogLevelInfo
	case "warn", "warning":
		return LogLevelWarn
	case "error":
		return LogLevelError
	case "off", "none":
		return LogLevelOff
	default:
		return LogLevelInfo
	}
}

func (l LogLevel) String() string {
	switch l {
	case LogLevelDebug:
		return "DEBUG"
	case LogLevelInfo:
		return "INFO"
	case LogLevelWarn:
		return "WARN"
	case LogLevelError:
		return "ERROR"
	case LogLevelOff:
		return "OFF"
	default:
		return "UNKNOWN"
	}
}

// LogRotateConfig 日志轮转配置
type LogRotateConfig struct {
	MaxSize    int // 单个日志文件最大大小（MB）
	MaxBackups int // 保留的旧日志文件数
	MaxAge     int // 保留的旧日志天数
	Compress   bool // 是否压缩旧日志
}

// DefaultLogRotateConfig 默认轮转配置
var DefaultLogRotateConfig = LogRotateConfig{
	MaxSize:    100,  // 100 MB
	MaxBackups: 7,    // 保留 7 个备份
	MaxAge:     28,   // 保留 28 天
	Compress:   true, // gzip 压缩
}

// LevelLogger implements Logger with level filtering and optional file output
type LevelLogger struct {
	level   LogLevel
	writer  io.Writer
	lj      *lumberjack.Logger // 保留引用以便 Close
	mu      sync.Mutex
}

// NewLevelLogger creates a logger with the given level and optional log file path.
// If logFile is empty, logs to stdout.
// rotate config controls log rotation; nil uses DefaultLogRotateConfig.
func NewLevelLogger(level string, logFile string, rotate *LogRotateConfig) (*LevelLogger, error) {
	l := &LevelLogger{
		level: parseLogLevel(level),
	}

	if logFile != "" {
		rc := DefaultLogRotateConfig
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

func (l *LevelLogger) logf(level LogLevel, format string, v ...interface{}) {
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

func (l *LevelLogger) Printf(format string, v ...interface{}) {
	// Auto-detect level from format string content
	level := LogLevelInfo
	lower := strings.ToLower(fmt.Sprintf(format, v...))
	if strings.Contains(lower, "debug") {
		level = LogLevelDebug
	} else if strings.Contains(lower, "warning") || strings.Contains(lower, "warn") {
		level = LogLevelWarn
	} else if strings.Contains(lower, "error") || strings.Contains(lower, "fail") {
		level = LogLevelError
	}
	l.logf(level, format, v...)
}

func (l *LevelLogger) Debug(args ...interface{}) {
	l.logf(LogLevelDebug, "%s", fmt.Sprint(args...))
}

func (l *LevelLogger) Debugf(format string, v ...interface{}) {
	l.logf(LogLevelDebug, format, v...)
}

func (l *LevelLogger) Info(args ...interface{}) {
	l.logf(LogLevelInfo, "%s", fmt.Sprint(args...))
}

func (l *LevelLogger) Infof(format string, v ...interface{}) {
	l.logf(LogLevelInfo, format, v...)
}

func (l *LevelLogger) Warn(args ...interface{}) {
	l.logf(LogLevelWarn, "%s", fmt.Sprint(args...))
}

func (l *LevelLogger) Warnf(format string, v ...interface{}) {
	l.logf(LogLevelWarn, format, v...)
}

func (l *LevelLogger) Error(args ...interface{}) {
	l.logf(LogLevelError, "%s", fmt.Sprint(args...))
}

func (l *LevelLogger) Errorf(format string, v ...interface{}) {
	l.logf(LogLevelError, format, v...)
}

// Rotate triggers an immediate log rotation
func (l *LevelLogger) Rotate() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.lj != nil {
		return l.lj.Rotate()
	}
	return nil
}

// Close closes the log file if one was opened
func (l *LevelLogger) Close() error {
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
