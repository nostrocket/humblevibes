package utils

import (
	"fmt"
	"log"
	"strings"
)

// LogLevel represents the severity of a log message
type LogLevel int

const (
	// DEBUG level for verbose development messages
	DEBUG LogLevel = iota
	// INFO level for general information
	INFO
	// WARNING level for important but non-critical messages
	WARNING
	// ERROR level for critical issues
	ERROR
)

// Logger provides a structured logging interface with component tracking
type Logger struct {
	Component string
	Level     LogLevel
}

// NewLogger creates a new logger for a specific component
func NewLogger(component string) *Logger {
	return &Logger{
		Component: component,
		Level:     INFO, // Default to INFO level
	}
}

// SetLevel sets the minimum log level for this logger
func (l *Logger) SetLevel(level LogLevel) {
	l.Level = level
}

// formatMessage formats a log message with the component prefix
func (l *Logger) formatMessage(level LogLevel, format string, args ...interface{}) string {
	var levelStr string
	var prefix string

	switch level {
	case DEBUG:
		levelStr = "DEBUG"
		prefix = "🔍"
	case INFO:
		levelStr = "INFO"
		prefix = "ℹ️"
	case WARNING:
		levelStr = "WARN"
		prefix = "⚠️"
	case ERROR:
		levelStr = "ERROR"
		prefix = "❌"
	}

	// Remove existing emoji prefixes if present
	if strings.HasPrefix(format, "✅") || strings.HasPrefix(format, "❌") || 
	   strings.HasPrefix(format, "🧪") || strings.HasPrefix(format, "🔍") || 
	   strings.HasPrefix(format, "ℹ️") || strings.HasPrefix(format, "⚠️") {
		format = format[len("✅"):]
	}

	// Format the message with arguments
	var message string
	if len(args) > 0 {
		message = fmt.Sprintf(format, args...)
	} else {
		message = format
	}

	// Return formatted string with component and level
	return fmt.Sprintf("%s [%s:%s] %s", prefix, l.Component, levelStr, message)
}

// Debug logs a message at DEBUG level
func (l *Logger) Debug(format string, args ...interface{}) {
	if l.Level <= DEBUG {
		log.Print(l.formatMessage(DEBUG, format, args...))
	}
}

// Info logs a message at INFO level
func (l *Logger) Info(format string, args ...interface{}) {
	if l.Level <= INFO {
		log.Print(l.formatMessage(INFO, format, args...))
	}
}

// Warn logs a message at WARNING level
func (l *Logger) Warn(format string, args ...interface{}) {
	if l.Level <= WARNING {
		log.Print(l.formatMessage(WARNING, format, args...))
	}
}

// Error logs a message at ERROR level
func (l *Logger) Error(format string, args ...interface{}) {
	if l.Level <= ERROR {
		log.Print(l.formatMessage(ERROR, format, args...))
	}
}

// Print directly prints a formatted message to stdout with component information
// Useful for test output that should not go through log
func (l *Logger) Print(format string, args ...interface{}) {
	fmt.Print(l.formatMessage(INFO, format, args...))
}

// Println directly prints a formatted message to stdout with component information and a newline
func (l *Logger) Println(format string, args ...interface{}) {
	fmt.Println(l.formatMessage(INFO, format, args...))
}

// TestInfo is specifically for test output formatting
func (l *Logger) TestInfo(format string, args ...interface{}) {
	// For tests, we want to use a special format to keep the emoji but add component info
	var message string
	if len(args) > 0 {
		message = fmt.Sprintf(format, args...)
	} else {
		message = format
	}
	fmt.Printf("[%s] %s\n", l.Component, message)
}
