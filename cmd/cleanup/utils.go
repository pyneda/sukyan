package cleanup

import "fmt"

// formatBytes formats byte counts into human-readable strings
func formatBytes(bytes int) string {
	if bytes < 1024 {
		return fmt.Sprintf("%d B", bytes)
	} else if bytes < 1024*1024 {
		return fmt.Sprintf("%.1f KB", float64(bytes)/1024)
	} else if bytes < 1024*1024*1024 {
		return fmt.Sprintf("%.1f MB", float64(bytes)/(1024*1024))
	}
	return fmt.Sprintf("%.1f GB", float64(bytes)/(1024*1024*1024))
}

// formatNumber formats large numbers into human-readable strings
func formatNumber(num int64) string {
	if num < 1000 {
		return fmt.Sprintf("%d", num)
	} else if num < 1000000 {
		return fmt.Sprintf("%.1fK", float64(num)/1000)
	} else if num < 1000000000 {
		return fmt.Sprintf("%.1fM", float64(num)/1000000)
	}
	return fmt.Sprintf("%.1fB", float64(num)/1000000000)
}
