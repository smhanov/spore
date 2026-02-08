package blog

import (
	"fmt"
	"strings"
	"time"
)

const (
	dateDisplayAbsolute    = "absolute"
	dateDisplayApproximate = "approximate"
)

func normalizeDateDisplay(value string) string {
	value = strings.TrimSpace(strings.ToLower(value))
	switch value {
	case dateDisplayApproximate:
		return dateDisplayApproximate
	case dateDisplayAbsolute:
		return dateDisplayAbsolute
	default:
		return dateDisplayAbsolute
	}
}

func resolveBlogSettings(settings *BlogSettings) BlogSettings {
	if settings == nil {
		return BlogSettings{CommentsEnabled: true, DateDisplay: dateDisplayAbsolute}
	}
	resolved := *settings
	resolved.DateDisplay = normalizeDateDisplay(resolved.DateDisplay)
	return resolved
}

func formatPublishedDate(publishedAt *time.Time, dateDisplay string) string {
	if publishedAt == nil {
		return ""
	}
	mode := normalizeDateDisplay(dateDisplay)
	if mode == dateDisplayApproximate {
		delta := time.Since(*publishedAt)
		if delta < 0 {
			delta = -delta
		}
		return fmt.Sprintf("Published %s ago", humanizeApproxDuration(delta))
	}
	return fmt.Sprintf("Published %s", publishedAt.Format("Jan 2, 2006"))
}

func humanizeApproxDuration(delta time.Duration) string {
	seconds := int(delta.Seconds())
	if seconds < 60 {
		if seconds <= 1 {
			return "1 second"
		}
		return fmt.Sprintf("%d seconds", seconds)
	}
	minutes := int(delta.Minutes())
	if minutes < 60 {
		if minutes == 1 {
			return "1 minute"
		}
		return fmt.Sprintf("%d minutes", minutes)
	}
	hours := int(delta.Hours())
	if hours < 24 {
		if hours == 1 {
			return "1 hour"
		}
		return fmt.Sprintf("%d hours", hours)
	}
	days := hours / 24
	if days < 30 {
		if days == 1 {
			return "1 day"
		}
		return fmt.Sprintf("%d days", days)
	}
	months := days / 30
	if months < 12 {
		if months == 1 {
			return "1 month"
		}
		return fmt.Sprintf("%d months", months)
	}
	years := days / 365
	if years <= 1 {
		return "1 year"
	}
	return fmt.Sprintf("%d years", years)
}
