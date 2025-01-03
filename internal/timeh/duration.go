package timeh

import (
	"fmt"
	"math"
	"strings"
	"time"
)

func Duration(d time.Duration) string {
	if d == 0 {
		return "0s"
	}

	isNegative := math.Signbit(float64(d))
	if isNegative {
		d = -d
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
		if isNegative {
			return "-1s"
		}

		return "1s"
	}

	result = strings.TrimSpace(result)

	if isNegative {
		return "-" + result
	}

	return result
}
