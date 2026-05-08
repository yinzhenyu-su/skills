package driver

import (
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"time"
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

// LevelLogger implements Logger with level filtering and optional file output
type LevelLogger struct {
	level  LogLevel
	writer io.Writer
	mu     sync.Mutex
}

// NewLevelLogger creates a logger with the given level and optional log file path.
// If logFile is empty, logs to stdout.
func NewLevelLogger(level string, logFile string) (*LevelLogger, error) {
	l := &LevelLogger{
		level: parseLogLevel(level),
	}

	if logFile != "" {
		f, err := os.OpenFile(logFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		if err != nil {
			return nil, fmt.Errorf("failed to open log file %s: %w", logFile, err)
		}
		l.writer = f
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
	if strings.Contains(lower, "debug") || strings.Contains(lower, "[dbg]") {
		level = LogLevelDebug
	} else if strings.Contains(lower, "warning") || strings.Contains(lower, "warn") {
		level = LogLevelWarn
	} else if strings.Contains(lower, "error") || strings.Contains(lower, "fail") {
		level = LogLevelError
	}
	l.logf(level, format, v...)
}

func (l *LevelLogger) Debugf(format string, v ...interface{}) {
	l.logf(LogLevelDebug, format, v...)
}

func (l *LevelLogger) Infof(format string, v ...interface{}) {
	l.logf(LogLevelInfo, format, v...)
}

func (l *LevelLogger) Warnf(format string, v ...interface{}) {
	l.logf(LogLevelWarn, format, v...)
}

func (l *LevelLogger) Errorf(format string, v ...interface{}) {
	l.logf(LogLevelError, format, v...)
}

// Close closes the log file if one was opened
func (l *LevelLogger) Close() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if c, ok := l.writer.(io.Closer); ok {
		return c.Close()
	}
	return nil
}
