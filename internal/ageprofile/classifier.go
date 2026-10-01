// Package ageprofile groups modification times by elapsed whole 24-hour days.
// It is a deterministic threshold classifier, not a language model.
package ageprofile

import (
	"strconv"
	"strings"
	"time"
)

const day = 24 * time.Hour

// Label classifies one modification time relative to the current time.
func Label(modified time.Time) string { return LabelAt(modified, time.Now()) }

// LabelAt makes boundaries reproducible. Future timestamps count as fresh.
// Duration saturation for very old dates is harmless: they remain archived.
func LabelAt(modified, now time.Time) string {
	days := now.Sub(modified) / day
	switch {
	case days < 7:
		return "fresh"
	case days < 31:
		return "recent"
	case days < 366:
		return "aging"
	default:
		return "archived"
	}
}

// Classify takes one clock snapshot for the whole batch.
func Classify(modified []time.Time) map[string]int { return ClassifyAt(modified, time.Now()) }

func ClassifyAt(modified []time.Time, now time.Time) map[string]int {
	counts := make(map[string]int, 4)
	for _, stamp := range modified {
		counts[LabelAt(stamp, now)]++
	}
	return counts
}

// Format omits zero counts and returns an empty string for an empty profile.
func Format(counts map[string]int) string {
	parts := make([]string, 0, 4)
	for _, label := range []string{"fresh", "recent", "aging", "archived"} {
		if counts[label] > 0 {
			parts = append(parts, label+" "+strconv.Itoa(counts[label]))
		}
	}
	if len(parts) == 0 {
		return ""
	}
	return "Age profile: " + strings.Join(parts, " | ")
}
