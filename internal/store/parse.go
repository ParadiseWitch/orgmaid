package store

import (
	"regexp"
	"sort"
	"strings"
	"time"
)

var (
	dateHeaderRe = regexp.MustCompile(`^\* ([0-9]{4}-[0-9]{2}-[0-9]{2})$`)
	itemRe       = regexp.MustCompile(`^\*\* (?:(TODO|DONE) )?(.*?)(\s+:[a-zA-Z0-9_]+(?::[a-zA-Z0-9_]+)*:)?$`)
	// CLOCK: [2026-09-18 四 09:00]--[2026-09-18 四 10:30]
	clockRe = regexp.MustCompile(`^\s+CLOCK:\s*\[([^\]]+)\](?:--\[([^\]]+)\])?\s*$`)
	// SCHEDULED: <2026-09-21 六>
	scheduledRe = regexp.MustCompile(`^\s+SCHEDULED:\s*<([^>]+)>\s*$`)
	// DEADLINE: <2026-09-22 日>
	deadlineRe = regexp.MustCompile(`^\s+DEADLINE:\s*<([^>]+)>\s*$`)
	// Legacy format: - START: 09:00 / - END: 10:30
	fieldRe = regexp.MustCompile(`(?i)^   - (START|END)\s*:\s*([0-9]{1,2}:[0-9]{2})\s*$`)
)

// ParseDate normalises a compact or dashed date to DateLayout.
func ParseDate(s string) (string, bool) {
	for _, layout := range []string{HeaderDateLayout, DateLayout} {
		if parsed, err := time.Parse(layout, s); err == nil {
			return parsed.Format(DateLayout), true
		}
	}
	return "", false
}

// Parse reads org-mode into a journal. Days come back in ascending date order
// and repeated date headers are merged. Lines the format does not describe are
// dropped, since orgmaid owns the file outright.
func Parse(data []byte) Journal {
	var j Journal
	dayIdx, itemIdx := -1, -1

	for _, raw := range strings.Split(string(data), "\n") {
		line := strings.TrimRight(raw, "\r")

		if m := dateHeaderRe.FindStringSubmatch(line); m != nil {
			if _, ok := ParseDate(m[1]); !ok {
				dayIdx, itemIdx = -1, -1
				continue
			}
			dayIdx, itemIdx = j.appendDay(m[1]), -1
			continue
		}

		if dayIdx < 0 {
			continue
		}

		if m := itemRe.FindStringSubmatch(line); m != nil {
			item := Item{Content: strings.TrimSpace(m[2])}
			if m[1] != "" {
				item.Todo = m[1]
			}
			if m[3] != "" {
				item.Tags = parseTags(m[3])
			}
			j.Days[dayIdx].Items = append(j.Days[dayIdx].Items, item)
			itemIdx = len(j.Days[dayIdx].Items) - 1
			continue
		}

		if itemIdx < 0 {
			continue
		}

		item := &j.Days[dayIdx].Items[itemIdx]
		date := j.Days[dayIdx].Date

		// Try CLOCK format first
		if m := clockRe.FindStringSubmatch(line); m != nil {
			if start, ok := ParseTimestamp(m[1]); ok {
				item.Start = &start
			}
			if m[2] != "" {
				if end, ok := ParseTimestamp(m[2]); ok {
					item.End = &end
				}
			}
			continue
		}

		// Try SCHEDULED
		if m := scheduledRe.FindStringSubmatch(line); m != nil {
			if ts, ok := ParseTimestamp(m[1]); ok {
				item.Scheduled = &ts
			}
			continue
		}

		// Try DEADLINE
		if m := deadlineRe.FindStringSubmatch(line); m != nil {
			if ts, ok := ParseTimestamp(m[1]); ok {
				item.Deadline = &ts
			}
			continue
		}

		// Fall back to legacy START/END format
		if m := fieldRe.FindStringSubmatch(line); m != nil {
			t, ok := ParseTime(m[2])
			if !ok || !t.Valid() {
				continue
			}
			ts, ok := TimestampFromDate(date, t)
			if !ok {
				continue
			}
			if strings.EqualFold(m[1], "START") {
				item.Start = &ts
			} else {
				item.End = &ts
			}
		}
	}

	j.mergeDuplicateDays()
	return j
}

// appendDay returns the index of the day a header line belongs to. Days arrive
// in file order, so a header that continues the run is reused or appended; one
// that jumps backwards defers to mergeDuplicateDays.
func (j *Journal) appendDay(date string) int {
	if n := len(j.Days); n > 0 && j.Days[n-1].Date == date {
		return n - 1
	}
	j.Days = append(j.Days, Day{Date: date})
	return len(j.Days) - 1
}

func (j *Journal) mergeDuplicateDays() {
	sort.SliceStable(j.Days, func(a, b int) bool { return j.Days[a].Date < j.Days[b].Date })

	merged := j.Days[:0]
	for _, day := range j.Days {
		if n := len(merged); n > 0 && merged[n-1].Date == day.Date {
			merged[n-1].Items = append(merged[n-1].Items, day.Items...)
			continue
		}
		merged = append(merged, day)
	}
	j.Days = merged
}

// parseTags extracts tags from a string like " :tag1:tag2:".
func parseTags(s string) []string {
	s = strings.TrimSpace(s)
	parts := strings.Split(s, ":")
	var tags []string
	for _, p := range parts {
		if p != "" {
			tags = append(tags, p)
		}
	}
	return tags
}
