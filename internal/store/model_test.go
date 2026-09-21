package store

import (
	"testing"
	"time"
)

func TestAddDurationWritesTheEndTime(t *testing.T) {
	cases := []struct {
		name        string
		startDate   string
		startH      int
		minutes     int
		wantEndDate string
		wantEndH    int
		wantEndM    int
		wantDur     time.Duration
	}{
		{"a plain span", "2026-08-01", 9, 70, "2026-08-01", 10, 10, 70 * time.Minute},
		{"no span at all", "2026-08-01", 9, 0, "2026-08-01", 9, 0, 0},
		{"up to midnight", "2026-08-01", 23, 60, "2026-08-02", 0, 0, time.Hour},
		{"past midnight", "2026-08-01", 23, 120, "2026-08-02", 1, 0, 2 * time.Hour},
		{"a whole day lands back on the start", "2026-08-01", 9, 24 * 60, "2026-08-02", 9, 0, 24 * time.Hour},
		{"negative span becomes zero", "2026-08-01", 9, -30, "2026-08-01", 9, 0, 0},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			start := tsPtrHelper(c.startDate, c.startH, 0)
			item := Item{Content: "记录", Start: start}

			if !item.AddDuration(c.minutes) {
				t.Fatalf("AddDuration(%d) = false, want true", c.minutes)
			}
			if item.End == nil {
				t.Fatal("no end time was written")
			}
			if item.End.Year != 2026 || item.End.Month != 8 {
				t.Errorf("end date = %04d-%02d-%02d, want 2026-08", item.End.Year, item.End.Month, item.End.Day)
			}
			if item.End.Day != parseDay(c.wantEndDate) {
				t.Errorf("end day = %d, want %d", item.End.Day, parseDay(c.wantEndDate))
			}
			if item.End.Hour != c.wantEndH || item.End.Minute != c.wantEndM {
				t.Errorf("end time = %02d:%02d, want %02d:%02d", item.End.Hour, item.End.Minute, c.wantEndH, c.wantEndM)
			}
			dur, _ := item.Duration()
			if dur != c.wantDur {
				t.Errorf("Duration() = %v; want %v", dur, c.wantDur)
			}
			if item.Start == item.End {
				t.Error("the new end time shares its pointer with the start time")
			}
		})
	}
}

func parseDay(date string) int {
	ts, _ := ParseDate(date)
	if ts == "" {
		return 0
	}
	// date is "2006-01-02", day is last 2 chars
	return int(ts[8]-'0')*10 + int(ts[9]-'0')
}

func TestAddDurationWithoutAStartTime(t *testing.T) {
	end := tsPtrHelper("2026-08-01", 18, 0)
	item := Item{Content: "还没开始", End: end}

	if item.AddDuration(60) {
		t.Error("AddDuration(60) = true, want false when there is no start to measure from")
	}
	if item.End.Hour != 18 || item.End.Minute != 0 {
		t.Errorf("the end time was rewritten to %02d:%02d, want the row left alone", item.End.Hour, item.End.Minute)
	}

	// An end time is not invented either, so the row still reads as no span.
	var bare Item
	if bare.AddDuration(60) {
		t.Error("AddDuration(60) = true on an empty item, want false")
	}
	if bare.End != nil {
		t.Errorf("end = %v, want it left unset", bare.End)
	}
}

func tsPtrHelper(date string, h, m int) *Timestamp {
	ts, _ := TimestampFromDateTime(date, h, m)
	return &ts
}
