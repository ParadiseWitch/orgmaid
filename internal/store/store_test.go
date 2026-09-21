package store

import (
	"reflect"
	"testing"
	"time"
)

func tsPtr(date string, h, m int) *Timestamp {
	ts, _ := TimestampFromDateTime(date, h, m)
	return &ts
}

func TestParseSpecExample(t *testing.T) {
	input := `* 2026-08-01
** 日志事项1内容
   CLOCK: [2026-08-01 六 09:00]--[2026-08-01 六 10:21]
** 日志事项2内容
   CLOCK: [2026-08-01 六 10:30]--[2026-08-01 六 11:21]
`

	got := Parse([]byte(input))
	want := Journal{Days: []Day{{
		Date: "2026-08-01",
		Items: []Item{
			{Content: "日志事项1内容", Start: tsPtr("2026-08-01", 9, 0), End: tsPtr("2026-08-01", 10, 21)},
			{Content: "日志事项2内容", Start: tsPtr("2026-08-01", 10, 30), End: tsPtr("2026-08-01", 11, 21)},
		},
	}}}

	if !reflect.DeepEqual(got, want) {
		t.Errorf("Parse mismatch\n got: %+v\nwant: %+v", got, want)
	}
}

func TestParseLegacyFormat(t *testing.T) {
	input := `* 2026-08-01
** 日志事项1内容
   - START: 09:00
   - END: 10:21
** 日志事项2内容
   - START: 10:30
   - END: 11:21
`

	got := Parse([]byte(input))
	want := Journal{Days: []Day{{
		Date: "2026-08-01",
		Items: []Item{
			{Content: "日志事项1内容", Start: tsPtr("2026-08-01", 9, 0), End: tsPtr("2026-08-01", 10, 21)},
			{Content: "日志事项2内容", Start: tsPtr("2026-08-01", 10, 30), End: tsPtr("2026-08-01", 11, 21)},
		},
	}}}

	if !reflect.DeepEqual(got, want) {
		t.Errorf("Parse mismatch\n got: %+v\nwant: %+v", got, want)
	}
}

func TestParseToleratesVariants(t *testing.T) {
	input := "* 2026-08-01\r\n" +
		"** 内容正常\r\n" +
		"   - start:9:05\r\n" +
		"   - END: 17:45\r\n" +
		"这一行不属于格式，应当被丢弃\r\n" +
		"** 只有内容没有时间\r\n"

	got := Parse([]byte(input))
	want := Journal{Days: []Day{{
		Date: "2026-08-01",
		Items: []Item{
			{Content: "内容正常", Start: tsPtr("2026-08-01", 9, 5), End: tsPtr("2026-08-01", 17, 45)},
			{Content: "只有内容没有时间"},
		},
	}}}

	if !reflect.DeepEqual(got, want) {
		t.Errorf("Parse mismatch\n got: %+v\nwant: %+v", got, want)
	}
}

func TestParseRejectsImpossibleValues(t *testing.T) {
	input := `* 2026-13-40
** 日期不存在的段落整体忽略
* 2026-08-01
   CLOCK: [2026-08-01 六 09:00]
** 时间字段出现在条目之前，忽略
** 非法时间
   CLOCK: [2026-08-01 六 25:00]--[2026-08-01 六 10:99]
`

	got := Parse([]byte(input))
	want := Journal{Days: []Day{{
		Date: "2026-08-01",
		Items: []Item{
			{Content: "时间字段出现在条目之前，忽略"},
			{Content: "非法时间"},
		},
	}}}

	if !reflect.DeepEqual(got, want) {
		t.Errorf("Parse mismatch\n got: %+v\nwant: %+v", got, want)
	}
}

func TestParseSortsAscendingAndMergesDuplicates(t *testing.T) {
	input := `* 2026-08-03
** 三天
* 2026-08-01
** 一天
* 2026-08-03
** 三天续
* 2026-08-02
** 两天
`

	got := Parse([]byte(input))

	var dates []string
	for _, d := range got.Days {
		dates = append(dates, d.Date)
	}
	if want := []string{"2026-08-01", "2026-08-02", "2026-08-03"}; !reflect.DeepEqual(dates, want) {
		t.Fatalf("dates = %v, want %v", dates, want)
	}

	merged := got.Days[2].Items
	if len(merged) != 2 || merged[0].Content != "三天" || merged[1].Content != "三天续" {
		t.Errorf("duplicate day not merged in order: %+v", merged)
	}
}

func TestParseEmpty(t *testing.T) {
	if got := Parse(nil); len(got.Days) != 0 {
		t.Errorf("Parse(nil) = %+v, want empty journal", got)
	}
	if got := Parse([]byte("随手写的散文\n没有日期头\n")); len(got.Days) != 0 {
		t.Errorf("Parse of headerless text = %+v, want empty journal", got)
	}
}

func TestSerializeMatchesSpecLayout(t *testing.T) {
	j := Journal{Days: []Day{{
		Date: "2026-08-01",
		Items: []Item{
			{Content: "日志事项1内容", Start: tsPtr("2026-08-01", 9, 0), End: tsPtr("2026-08-01", 10, 21)},
			{Content: "日志事项2内容", Start: tsPtr("2026-08-01", 10, 30), End: tsPtr("2026-08-01", 11, 21)},
		},
	}}}

	want := `* 2026-08-01
** 日志事项1内容
   CLOCK: [2026-08-01 六 09:00]--[2026-08-01 六 10:21]
** 日志事项2内容
   CLOCK: [2026-08-01 六 10:30]--[2026-08-01 六 11:21]
`

	if got := string(j.Serialize()); got != want {
		t.Errorf("Serialize mismatch\n got:\n%s\nwant:\n%s", got, want)
	}
}

func TestSerializeOmitsMissingFields(t *testing.T) {
	j := Journal{Days: []Day{
		{Date: "2026-08-01", Items: []Item{
			{Content: "只有开始", Start: tsPtr("2026-08-01", 9, 0)},
			{Content: "只有结束", End: tsPtr("2026-08-01", 18, 30)},
			{Content: "都没有"},
		}},
		{Date: "2026-08-02"},
	}}

	want := `* 2026-08-01
** 只有开始
   CLOCK: [2026-08-01 六 09:00]
** 只有结束
** 都没有

* 2026-08-02
`

	if got := string(j.Serialize()); got != want {
		t.Errorf("Serialize mismatch\n got:\n%s\nwant:\n%s", got, want)
	}
}

func TestRoundTripIsStable(t *testing.T) {
	inputs := []string{
		`* 2026-08-01
** 日志事项1内容
   CLOCK: [2026-08-01 六 09:00]--[2026-08-01 六 10:21]
** 日志事项2内容
   CLOCK: [2026-08-01 六 10:30]--[2026-08-01 六 11:21]
`,
		`* 2026-08-01
** 只有开始
   CLOCK: [2026-08-01 六 09:00]
** 只有结束
** 都没有

* 2026-08-02
`,
		`* 2026-01-01
** 跨年
   CLOCK: [2026-01-01 四 23:00]--[2026-01-02 五 01:30]
`,
	}

	for _, input := range inputs {
		once := Parse([]byte(input))
		twice := Parse(once.Serialize())

		if !reflect.DeepEqual(once, twice) {
			t.Fatalf("round trip changed the model\nfirst:  %+v\nsecond: %+v", once, twice)
		}
		if got, want := string(once.Serialize()), string(twice.Serialize()); got != want {
			t.Errorf("round trip changed the bytes\nfirst:\n%s\nsecond:\n%s", got, want)
		}
	}
}

func TestDurationCrossesMidnight(t *testing.T) {
	cases := []struct {
		name       string
		start, end *Timestamp
		want       time.Duration
		crossed    bool
		shown      bool
	}{
		{"same day", tsPtr("2026-08-01", 9, 0), tsPtr("2026-08-01", 10, 10), 70 * time.Minute, false, true},
		{"next day is not crossing", tsPtr("2026-08-01", 23, 0), tsPtr("2026-08-02", 1, 0), 2 * time.Hour, false, true},
		{"same time zero length", tsPtr("2026-08-01", 9, 0), tsPtr("2026-08-01", 9, 0), 0, false, true},
		{"no start", nil, tsPtr("2026-08-01", 9, 0), 0, false, false},
		{"no end", tsPtr("2026-08-01", 9, 0), nil, 0, false, false},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			item := Item{Start: c.start, End: c.end}
			got, crossed := item.Duration()

			if c.shown {
				if got != c.want || crossed != c.crossed {
					t.Errorf("Duration() = %v, %v; want %v, %v", got, crossed, c.want, c.crossed)
				}
			} else if got != 0 || crossed {
				t.Errorf("Duration() = %v, %v; want 0, false", got, crossed)
			}
		})
	}
}

func TestFormatDuration(t *testing.T) {
	cases := map[time.Duration]string{
		0:                            "00h00m",
		70 * time.Minute:             "01h10m",
		10 * time.Minute:             "00h10m",
		26*time.Hour + 5*time.Minute: "26h05m",
		2*time.Hour + 59*time.Minute: "02h59m",
	}
	for in, want := range cases {
		if got := FormatDuration(in); got != want {
			t.Errorf("FormatDuration(%v) = %q, want %q", in, got, want)
		}
	}
}

func TestDayTotals(t *testing.T) {
	day := Day{Date: "2026-08-01", Items: []Item{
		{Content: "a", Start: tsPtr("2026-08-01", 9, 0), End: tsPtr("2026-08-01", 10, 0)},
		{Content: "b", Start: tsPtr("2026-08-01", 23, 0), End: tsPtr("2026-08-02", 1, 0)},
		{Content: "c", Start: tsPtr("2026-08-01", 14, 0)},
	}}

	// a = 1h, b = 2h (next day), c = no end
	if got, want := day.TotalDuration(), 3*time.Hour; got != want {
		t.Errorf("TotalDuration() = %v, want %v", got, want)
	}
	if got, want := day.Weekday(), "周六"; got != want {
		t.Errorf("Weekday() = %q, want %q", got, want)
	}
	if got, want := day.CompactDate(), "20260801"; got != want {
		t.Errorf("CompactDate() = %q, want %q", got, want)
	}
}

func TestJournalEnsureDayKeepsOrder(t *testing.T) {
	var j Journal

	for _, date := range []string{"2026-08-05", "2026-08-01", "2026-08-03", "2026-08-01"} {
		j.EnsureDay(date)
	}

	var dates []string
	for _, d := range j.Days {
		dates = append(dates, d.Date)
	}
	want := []string{"2026-08-01", "2026-08-03", "2026-08-05"}
	if !reflect.DeepEqual(dates, want) {
		t.Errorf("days = %v, want %v", dates, want)
	}

	if got := j.FindDay("2026-08-03"); got != 1 {
		t.Errorf("FindDay(2026-08-03) = %d, want 1", got)
	}
	if got := j.FindDay("2026-09-09"); got != -1 {
		t.Errorf("FindDay(absent) = %d, want -1", got)
	}
}

func TestParseDate(t *testing.T) {
	valid := map[string]string{
		"20260801":   "2026-08-01",
		"2026-08-01": "2026-08-01",
	}
	for in, want := range valid {
		got, ok := ParseDate(in)
		if !ok || got != want {
			t.Errorf("ParseDate(%q) = %q, %v; want %q, true", in, got, ok, want)
		}
	}
	for _, in := range []string{"2026-0801", "20261301", "not-a-date", "", "2026-02-30"} {
		if got, ok := ParseDate(in); ok {
			t.Errorf("ParseDate(%q) = %q, true; want false", in, got)
		}
	}
}

func TestSaveAndReload(t *testing.T) {
	path := t.TempDir() + "/nested/orgmaid.org"

	saved := &Store{Path: path}
	saved.Journal.EnsureDay("2026-08-02")
	day := saved.Day("2026-08-01")
	day.Items = append(day.Items, Item{Content: "写入再读回", Start: tsPtr("2026-08-01", 9, 0), End: tsPtr("2026-08-01", 9, 45)})

	if err := saved.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}

	reloaded, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if !reflect.DeepEqual(reloaded.Journal, saved.Journal) {
		t.Errorf("reloaded journal differs\n got: %+v\nwant: %+v", reloaded.Journal, saved.Journal)
	}
}

func TestOpenMissingFileYieldsEmptyJournal(t *testing.T) {
	s, err := Open(t.TempDir() + "/does-not-exist.md")
	if err != nil {
		t.Fatalf("Open on missing file: %v", err)
	}
	if len(s.Journal.Days) != 0 {
		t.Errorf("journal = %+v, want empty", s.Journal)
	}
}
