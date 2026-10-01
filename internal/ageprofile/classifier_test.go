package ageprofile

import (
	"testing"
	"time"
)

func TestWholeDayBoundaries(t *testing.T) {
	now := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	for _, test := range []struct {
		age  time.Duration
		want string
	}{
		{-1000 * day, "fresh"}, {-time.Nanosecond, "fresh"}, {0, "fresh"}, {6 * day, "fresh"}, {7*day - time.Nanosecond, "fresh"},
		{7 * day, "recent"}, {30 * day, "recent"}, {31*day - time.Nanosecond, "recent"},
		{31 * day, "aging"}, {365 * day, "aging"}, {366*day - time.Nanosecond, "aging"}, {366 * day, "archived"}, {1000 * day, "archived"},
	} {
		if got := LabelAt(now.Add(-test.age), now); got != test.want {
			t.Errorf("age %v: got %s, want %s", test.age, got, test.want)
		}
	}
	if got := LabelAt(time.Time{}, now); got != "archived" {
		t.Fatal("duration saturation misclassified ancient date", got)
	}
}

func TestBatchCountsAndFormatting(t *testing.T) {
	now := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	stamps := []time.Time{now, now.Add(-6 * day), now.Add(-7 * day), now.Add(-31 * day), now.Add(-366 * day)}
	counts := ClassifyAt(stamps, now)
	if counts["fresh"] != 2 || counts["recent"] != 1 || counts["aging"] != 1 || counts["archived"] != 1 {
		t.Fatal(counts)
	}
	if got := Format(counts); got != "Age profile: fresh 2 | recent 1 | aging 1 | archived 1" {
		t.Fatal(got)
	}
	if got := Format(map[string]int{"aging": 3, "fresh": 0}); got != "Age profile: aging 3" {
		t.Fatal(got)
	}
	if Format(Classify(nil)) != "" {
		t.Fatal("empty directory produced summary")
	}
	if Label(time.Now().Add(-time.Hour)) != "fresh" {
		t.Fatal("single-file clock wrapper")
	}
	if Classify([]time.Time{time.Now()})["fresh"] != 1 {
		t.Fatal("batch clock wrapper")
	}
}

func TestElapsedDaysDoNotDependOnCalendarOrZone(t *testing.T) {
	start := time.Date(2024, 2, 29, 10, 0, 0, 0, time.FixedZone("UTC+5", 5*3600))
	now := start.Add(7 * day)
	if LabelAt(start, now) != "recent" || LabelAt(start.UTC(), now.UTC()) != "recent" {
		t.Fatal("zone changed elapsed age")
	}
	if LabelAt(start, now.Add(-time.Nanosecond)) != "fresh" {
		t.Fatal("calendar date replaced elapsed duration")
	}
}

func FuzzAgeBoundaries(f *testing.F) {
	for _, value := range []int64{-1, 0, 6, 7, 30, 31, 365, 366, 100000} {
		f.Add(value)
	}
	f.Fuzz(func(t *testing.T, seconds int64) {
		now := time.Unix(2000000000, 0)
		stamp := time.Unix(seconds, 0)
		label := LabelAt(stamp, now)
		if label != "fresh" && label != "recent" && label != "aging" && label != "archived" {
			t.Fatal(label)
		}
		if ClassifyAt([]time.Time{stamp, stamp}, now)[label] != 2 {
			t.Fatal("single/batch disagreement")
		}
	})
}
