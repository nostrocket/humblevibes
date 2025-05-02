package utils

import (
	"time"
)

// TruncateString truncates a string to the specified length
// and adds an ellipsis if it exceeds the maximum length
func TruncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	if maxLen <= 3 {
		return s[:maxLen]
	}
	return s[:maxLen-3] + "..."
}

// FormatTime formats a Unix timestamp into a human-readable string
func FormatTime(timestamp int64, format string) string {
	return time.Unix(timestamp, 0).Format(format)
}

// IsOlderThan checks if a timestamp is older than the specified duration
func IsOlderThan(timestamp int64, duration time.Duration) bool {
	return time.Now().Unix()-timestamp > int64(duration.Seconds())
}
