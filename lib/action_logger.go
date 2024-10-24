package lib

import (
	"fmt"
	"sync"
	"time"
)

// LogLevel represents the severity of a log entry
type LogLevel string

const (
	INFO  LogLevel = "INFO"
	WARN  LogLevel = "WARN"
	ERROR LogLevel = "ERROR"
)

// LogEntry represents a single log message
type LogEntry struct {
	Level     LogLevel  `json:"level"`
	Text      string    `json:"text"`
	Timestamp time.Time `json:"timestamp"`
}

// ActionLogger is a custom logger that captures log entries
type ActionLogger struct {
	entries []LogEntry
	mu      sync.Mutex
}

// NewActionLogger creates a new ActionLogger instance
func NewActionLogger() *ActionLogger {
	return &ActionLogger{
		entries: []LogEntry{},
	}
}

// Log adds a new log entry
func (l *ActionLogger) Log(level LogLevel, text string) error {
	l.mu.Lock()
	defer l.mu.Unlock()

	switch level {
	case INFO, WARN, ERROR:
		l.entries = append(l.entries, LogEntry{
			Level:     level,
			Text:      text,
			Timestamp: time.Now(),
		})
		return nil
	default:
		return fmt.Errorf("invalid log level: %s", level)
	}
}

// GetLogs returns the captured logs
func (l *ActionLogger) GetLogs() []LogEntry {
	l.mu.Lock()
	defer l.mu.Unlock()

	logsCopy := make([]LogEntry, len(l.entries))
	copy(logsCopy, l.entries)

	return logsCopy
}

// ClearLogs clears the captured logs
func (l *ActionLogger) ClearLogs() {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.entries = []LogEntry{}
}

// PrintLogs prints all logs to the console
func (l *ActionLogger) PrintLogs() {
	l.mu.Lock()
	defer l.mu.Unlock()
	for _, entry := range l.entries {
		fmt.Printf("[%s] %s: %s\n", entry.Timestamp.Format(time.RFC3339), entry.Level, entry.Text)
	}
}
