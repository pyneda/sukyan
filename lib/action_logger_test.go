package lib

import (
	"testing"
	"time"
)

func TestNewActionLogger(t *testing.T) {
	logger := NewActionLogger()
	if logger == nil {
		t.Error("NewActionLogger() returned nil")
	}
	if len(logger.entries) != 0 {
		t.Error("New logger should have no entries")
	}
}

func TestLog(t *testing.T) {
	logger := NewActionLogger()

	tests := []struct {
		level     LogLevel
		text      string
		expectErr bool
	}{
		{INFO, "Info message", false},
		{WARN, "Warning message", false},
		{ERROR, "Error message", false},
		{LogLevel("INVALID"), "Invalid level", true},
	}

	for _, tt := range tests {
		err := logger.Log(tt.level, tt.text)
		if (err != nil) != tt.expectErr {
			t.Errorf("Log(%v, %s) error = %v, expectErr %v", tt.level, tt.text, err, tt.expectErr)
		}
	}

	logs := logger.GetLogs()
	if len(logs) != 3 {
		t.Errorf("Expected 3 log entries, got %d", len(logs))
	}

	for i, log := range logs {
		if log.Level != tests[i].level || log.Text != tests[i].text {
			t.Errorf("Log entry %d does not match expected values", i)
		}
		if time.Since(log.Timestamp) > time.Second {
			t.Errorf("Log timestamp for entry %d is too old", i)
		}
	}
}

func TestGetLogs(t *testing.T) {
	logger := NewActionLogger()
	logger.Log(INFO, "Test message")

	logs := logger.GetLogs()
	if len(logs) != 1 {
		t.Errorf("Expected 1 log entry, got %d", len(logs))
	}
	if logs[0].Text != "Test message" || logs[0].Level != INFO {
		t.Error("Log entry does not match expected values")
	}
}

func TestClearLogs(t *testing.T) {
	logger := NewActionLogger()
	logger.Log(INFO, "Test message")
	logger.ClearLogs()

	logs := logger.GetLogs()
	if len(logs) != 0 {
		t.Errorf("Expected 0 log entries after clear, got %d", len(logs))
	}
}

func TestConcurrency(t *testing.T) {
	logger := NewActionLogger()
	const numGoroutines = 100
	const numLogsPerGoroutine = 100

	done := make(chan bool)

	for i := 0; i < numGoroutines; i++ {
		go func() {
			for j := 0; j < numLogsPerGoroutine; j++ {
				logger.Log(INFO, "Concurrent log message")
			}
			done <- true
		}()
	}

	for i := 0; i < numGoroutines; i++ {
		<-done
	}

	logs := logger.GetLogs()
	expectedLogs := numGoroutines * numLogsPerGoroutine
	if len(logs) != expectedLogs {
		t.Errorf("Expected %d log entries, got %d", expectedLogs, len(logs))
	}
}
