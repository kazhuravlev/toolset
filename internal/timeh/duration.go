package timeh

import (
	"fmt"
	"strings"
	"time"
)

func Duration(d time.Duration) string {
	if d == 0 {
		return "0s"
	}

	days := d / (24 * time.Hour)
	d -= days * 24 * time.Hour

	hours := d / time.Hour
	d -= hours * time.Hour

	minutes := d / time.Minute
	d -= minutes * time.Minute

	seconds := d / time.Second

	// Build the human-readable string
	var result string
	if days > 0 {
		result += fmt.Sprintf("%dd ", days)
	}

	if hours > 0 {
		result += fmt.Sprintf("%dh ", hours)
	}

	if minutes > 0 {
		result += fmt.Sprintf("%dm ", minutes)
	}

	if seconds > 0 {
		result += fmt.Sprintf("%ds ", seconds)
	}

	if result == "" {
		if d < 0 {
			return "-1s"
		}

		return "1s"
	}

	result = strings.TrimSpace(result)

	if d < 0 {
		return "-" + result
	}

	return result
}
