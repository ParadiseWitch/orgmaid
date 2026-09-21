package ui

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/muesli/termenv"

	"orgmaid/internal/config"
	"orgmaid/internal/keys"
	"orgmaid/internal/store"
)

const logDate = "2026-09-09"

// TestMain pins the colour profile. Test stdout is not a terminal, so left to
// itself lipgloss strips every escape sequence and the colour assertions below
// could not see anything; the colour tests opt in to TrueColor and put it back.
func TestMain(m *testing.M) {
	lipgloss.SetColorProfile(termenv.Ascii)
	os.Exit(m.Run())
}

// startLog builds a sized app over the given rows for logDate. The store file
// lives in the temp dir, because every mutation writes it.
func startLog(t *testing.T, items ...store.Item) *App {
	t.Helper()

	st := &store.Store{Path: filepath.Join(t.TempDir(), "orgmaid.org")}
	st.Journal.EnsureDay(logDate)
	st.Day(logDate).Items = append([]store.Item{}, items...)

	a := New(st, logDate, config.Default(), "")
	if _, cmd := a.Update(tea.WindowSizeMsg{Width: 80, Height: 24}); cmd != nil {
		t.Fatal("resizing the terminal scheduled a command")
	}
	return a
}

// rowOf makes one item. An empty clock string leaves that bound unset.
func rowOf(content, start, end string) store.Item {
	return store.Item{Content: content, Start: mustClock(start), End: mustClock(end)}
}

func mustClock(s string) *store.Timestamp {
	if s == "" {
		return nil
	}
	ts, ok := store.TimestampFromDateTime(logDate, parseHour(s), parseMinute(s))
	if !ok {
		panic("not a clock reading: " + s)
	}
	return &ts
}

func parseHour(s string) int {
	if len(s) < 2 {
		return 0
	}
	return int(s[0]-'0')*10 + int(s[1]-'0')
}

func parseMinute(s string) int {
	if len(s) < 5 {
		return 0
	}
	return int(s[3]-'0')*10 + int(s[4]-'0')
}

// stroke is one key press in a test script.
type stroke struct {
	r    rune // a typed character, or 0 when the key has no character
	name tea.KeyType
}

func ch(r rune) stroke { return stroke{r: r} }

func kt(name tea.KeyType) stroke { return stroke{name: name} }

// typed spreads a string over the keyboard, for scripts like "dd" or "18".
func typed(s string) []stroke {
	out := make([]stroke, 0, len(s))
	for _, r := range s {
		out = append(out, ch(r))
	}
	return out
}

// presses holds one named key down n times, for the keys that walk or step a
// number instead of typing a character.
func presses(name tea.KeyType, n int) []stroke {
	out := make([]stroke, n)
	for i := range out {
		out[i] = kt(name)
	}
	return out
}

func (s stroke) String() string {
	if s.r == 0 {
		return fmt.Sprintf("<key %d>", s.name)
	}
	return string(s.r)
}

func (s stroke) msg() tea.KeyMsg {
	if s.r == 0 {
		return tea.KeyMsg{Type: s.name}
	}
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{s.r}}
}

// send presses the keys in order and returns the command the last one asked for.
func send(t *testing.T, a *App, keys ...stroke) tea.Cmd {
	t.Helper()

	var cmd tea.Cmd
	for _, k := range keys {
		_, cmd = a.Update(k.msg())
	}
	return cmd
}

// park walks the row cursor onto one stop with the keys a user would press —
// the shorter way round the ring in Tab or shift+Tab — rather than assigning the
// stop. Every test that parks therefore also proves the ring is walked the way it
// is drawn, and a stopped cursor that arrived by some other route is caught here.
func park(t *testing.T, a *App, stop int) {
	t.Helper()

	on := a.log.field
	forward, back := wrap(stop-on, stopCount), wrap(on-stop, stopCount)
	switch {
	case on == stop:
		return
	case back < forward:
		send(t, a, presses(tea.KeyShiftTab, back)...)
	default:
		send(t, a, presses(tea.KeyTab, forward)...)
	}

	if a.log.field != stop {
		t.Fatalf("walking from %s landed on %s, want %s",
			stopNames[on], stopNames[a.log.field], stopNames[stop])
	}
}

// rowState is everything an item can carry, on one line: a failure then says
// what moved rather than printing two pointers.
func rowState(a *App, i int) string {
	it := a.items()[i]

	dur := "--"
	if it.Start != nil && it.End != nil {
		d, _ := it.Duration()
		dur = store.FormatDuration(d)
	}
	return fmt.Sprintf("%q start=%s end=%s dur=%s", it.Content, timeText(it.Start), timeText(it.End), dur)
}

func timeText(x *store.Timestamp) string {
	if x == nil {
		return "unset"
	}
	return fmt.Sprintf("%02d:%02d", x.Hour, x.Minute)
}

// wantRows checks the whole day, row by row, so an inserted or reordered row
// shows up as a length mismatch instead of a missed assertion.
func wantRows(t *testing.T, a *App, want ...string) {
	t.Helper()

	if len(want) != len(a.items()) {
		t.Fatalf("day holds %d rows, want %d (%v)", len(a.items()), len(want), allRows(a))
	}
	for i, w := range want {
		if got := rowState(a, i); got != w {
			t.Errorf("row %d = %s, want %s", i, got, w)
		}
	}
}

func allRows(a *App) []string {
	out := make([]string, len(a.items()))
	for i := range out {
		out[i] = rowState(a, i)
	}
	return out
}

// wantFile checks the whole journal against what is on disk, so a keystroke that
// updated the model but never saved is caught.
func wantFile(t *testing.T, a *App) {
	t.Helper()

	reloaded, err := store.Open(a.store.Path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if !reflect.DeepEqual(reloaded.Journal, a.store.Journal) {
		t.Errorf("the file disagrees with the model\n file: %+v\nmodel: %+v", reloaded.Journal, a.store.Journal)
	}
}

// fileText is the store file as it stands, or empty before the first save. A test
// hands a refused keystroke the bytes it saw beforehand, so a dropped digit
// cannot have written anything.
func fileText(t *testing.T, a *App) string {
	t.Helper()

	out, err := os.ReadFile(a.store.Path)
	if errors.Is(err, os.ErrNotExist) {
		return ""
	}
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	return string(out)
}

// frameRows is the list region of a rendered page: the frame opens with a title
// and a rule, and the rows run down to the next rule.
func frameRows(t *testing.T, a *App) []string {
	t.Helper()

	lines := strings.Split(a.viewLog(), "\n")
	if len(lines) < 3 {
		t.Fatalf("frame has %d lines, want a title, a rule and a row", len(lines))
	}
	return lines[2:]
}

// isQuit reports whether the command ends the program.
func isQuit(t *testing.T, cmd tea.Cmd) bool {
	t.Helper()

	if cmd == nil {
		return false
	}
	_, ok := cmd().(tea.QuitMsg)
	return ok
}

func TestStopsAreNumberedAsTheModelAssumes(t *testing.T) {
	got := []int{fIndex, fTodo, fStartHour, fStartMinute, fEndHour, fEndMinute, fDurHour, fDurMinute, fContent, fScheduled, fDeadline}
	for i, s := range got {
		if s != i {
			t.Errorf("stop %d = %d, want %d", i, s, i)
		}
	}
	if stopCount != 11 {
		t.Errorf("stopCount = %d, want 11", stopCount)
	}
	if n := len(stopNames); n != stopCount {
		t.Errorf("stopNames holds %d names, want %d", n, stopCount)
	}

	for _, c := range []struct {
		name string
		stop int
		hour bool
	}{
		{"index", fIndex, false},
		{"todo", fTodo, false},
		{"start hour", fStartHour, true},
		{"start minute", fStartMinute, false},
		{"end hour", fEndHour, true},
		{"end minute", fEndMinute, false},
		{"duration hour", fDurHour, true},
		{"duration minute", fDurMinute, false},
		{"content", fContent, false},
		{"scheduled", fScheduled, false},
		{"deadline", fDeadline, false},
	} {
		if hour := hourStop(c.stop); hour != c.hour {
			t.Errorf("hourStop(%s) = %v, want %v", c.name, hour, c.hour)
		}
	}
	for _, c := range []struct {
		name string
		stop int
		time bool
	}{
		{"index", fIndex, false},
		{"todo", fTodo, false},
		{"content", fContent, false},
		{"scheduled", fScheduled, false},
		{"deadline", fDeadline, false},
		{"start hour", fStartHour, true},
		{"duration minute", fDurMinute, true},
	} {
		if isTime := timeStop(c.stop); isTime != c.time {
			t.Errorf("timeStop(%s) = %v, want %v", c.name, isTime, c.time)
		}
	}

	s := newLogState()
	if s.field != fContent {
		t.Errorf("a fresh page starts on %s, want the content stop", stopNames[s.field])
	}
	if s.editing {
		t.Error("a fresh page is already editing, want the editor closed")
	}
}

// TestStopsFormARing starts every case on a fresh page, where the cursor sits on
// the content stop, and touches only keys a user touches. Nothing here parks: a
// press of Tab has to land one stop forward on its own, because the cases that
// would otherwise agree in both directions do not exist in this table. The keys
// the redesign retired are covered by the last case: they must move nothing.
func TestStopsFormARing(t *testing.T) {
	cases := []struct {
		name string
		keys []stroke
		want int
	}{
		{"tab from the content stop reaches the scheduled", []stroke{kt(tea.KeyTab)}, fScheduled},
		{"shift+tab from the content stop backs into the duration minute", []stroke{kt(tea.KeyShiftTab)}, fDurMinute},
		{"one tab past the scheduled is the deadline", presses(tea.KeyTab, 2), fDeadline},
		{"two tabs past the scheduled is the index", presses(tea.KeyTab, 3), fIndex},
		{"one shift+tab past the duration minute is the duration hour", presses(tea.KeyShiftTab, 2), fDurHour},
		{"two tabs run down to the deadline", presses(tea.KeyTab, 2), fDeadline},
		{"an eleventh tab completes the lap", presses(tea.KeyTab, 11), fContent},
		{"eleven shift+tabs also complete the lap", presses(tea.KeyShiftTab, 11), fContent},
		{"shift+tab undoes tab", []stroke{kt(tea.KeyTab), kt(tea.KeyShiftTab)}, fContent},
		{"tab undoes shift+tab", []stroke{kt(tea.KeyShiftTab), kt(tea.KeyTab)}, fContent},
		{"0 picks the index stop from the content stop", typed("0"), fIndex},
		{"0 on the index stop stays on it", typed("00"), fIndex},
		{"Esc off a stop returns to the content", append(presses(tea.KeyTab, 3), kt(tea.KeyEsc)), fContent},
		{"Enter off a stop returns to the content", append(presses(tea.KeyShiftTab, 3), kt(tea.KeyEnter)), fContent},
		{"h on content moves to the previous stop", typed("h"), fDurMinute},
		{"l on content moves to the next stop", typed("l"), fScheduled},
		{"the left arrow no longer walks the row", []stroke{kt(tea.KeyLeft)}, fContent},
		{"the right arrow no longer walks the row", []stroke{kt(tea.KeyRight)}, fContent},
		{"h and l together move nothing", typed("hl"), fContent},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			a := startLog(t, rowOf("甲", "09:00", "10:00"))

			send(t, a, c.keys...)

			if a.log.field != c.want {
				t.Errorf("after %v: cursor on %s, want %s",
					c.keys, stopNames[a.log.field], stopNames[c.want])
			}
			if a.log.editing {
				t.Errorf("after %v: the editor is open, want it closed", c.keys)
			}
			wantRows(t, a, `"甲" start=09:00 end=10:00 dur=01h00m`)
		})
	}

	t.Run("the stops come in the order the columns are drawn", func(t *testing.T) {
		a := startLog(t, rowOf("甲", "09:00", "10:00"))
		order := []int{fScheduled, fDeadline, fIndex, fTodo, fStartHour, fStartMinute, fEndHour, fEndMinute, fDurHour, fDurMinute, fContent}

		for i, stop := range order {
			send(t, a, kt(tea.KeyTab))
			if a.log.field != stop {
				t.Fatalf("press %d of Tab: cursor on %s, want %s",
					i+1, stopNames[a.log.field], stopNames[stop])
			}
		}
		for i := len(order) - 2; i >= 0; i-- {
			send(t, a, kt(tea.KeyShiftTab))
			if a.log.field != order[i] {
				t.Fatalf("press %d of shift+Tab: cursor on %s, want %s",
					len(order)-1-i, stopNames[a.log.field], stopNames[order[i]])
			}
		}
		send(t, a, kt(tea.KeyShiftTab))
		if a.log.field != fContent {
			t.Errorf("one shift+Tab past the index stop: cursor on %s, want it to wrap to the content stop",
				stopNames[a.log.field])
		}
	})
}

// TestZeroPicksTheIndexStop covers the shortcut on the two stops that own it,
// and the six where 0 is instead the tens of a reading — where it rewrites the
// tens and nothing else.
func TestZeroPicksTheIndexStop(t *testing.T) {
	for _, stop := range []int{fIndex, fContent} {
		t.Run(stopNames[stop], func(t *testing.T) {
			a := startLog(t, rowOf("甲", "17:25", "13:40"))
			park(t, a, stop)

			send(t, a, ch('0'))

			if a.log.field != fIndex {
				t.Errorf("cursor on %s, want the index stop", stopNames[a.log.field])
			}
			wantRows(t, a, `"甲" start=17:25 end=13:40 dur=20h15m`)
		})
	}

	cases := []struct {
		stop int
		want string
	}{
		{fStartHour, `"甲" start=07:25 end=13:40 dur=06h15m`},
		{fStartMinute, `"甲" start=17:05 end=13:40 dur=20h35m`},
		{fEndHour, `"甲" start=17:25 end=03:40 dur=10h15m`},
		{fEndMinute, `"甲" start=17:25 end=13:00 dur=19h35m`},
		{fDurHour, `"甲" start=17:25 end=17:40 dur=00h15m`},
		{fDurMinute, `"甲" start=17:25 end=13:30 dur=20h05m`},
	}

	for _, c := range cases {
		t.Run(stopNames[c.stop], func(t *testing.T) {
			a := startLog(t, rowOf("甲", "17:25", "13:40"))
			park(t, a, c.stop)

			send(t, a, ch('0'))

			if a.log.field != c.stop {
				t.Errorf("cursor on %s, want it to keep the reading it is typing into",
					stopNames[a.log.field])
			}
			wantRows(t, a, c.want)
			if a.log.tensNext {
				t.Error("turn = the tens, want the zero to have been taken as the tens")
			}
			wantFile(t, a)
		})
	}
}

func TestDigitsStillJumpToAnItem(t *testing.T) {
	var many []store.Item
	for i := range 12 {
		many = append(many, rowOf(fmt.Sprintf("项%d", i+1), "09:00", "09:30"))
	}

	cases := []struct {
		name       string
		keys       string
		wantCursor int
	}{
		{"one digit", "1", 0},
		{"another digit", "3", 2},
		{"two digits reach item ten", "10", 9},
		{"a jump past the end clamps", "99", 11},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			a := startLog(t, many...)
			a.log.cursor = 5

			send(t, a, typed(c.keys)...)

			if a.log.cursor != c.wantCursor {
				t.Errorf("cursor = %d, want %d", a.log.cursor, c.wantCursor)
			}
		})
	}

	t.Run("a leading zero is the index stop, not a jump", func(t *testing.T) {
		a := startLog(t, many...)
		a.log.cursor = 5

		send(t, a, ch('0'))

		if a.log.cursor != 5 {
			t.Errorf("cursor = %d, want the row left where it was", a.log.cursor)
		}
		if a.log.field != fIndex {
			t.Errorf("cursor on %s, want the index stop", stopNames[a.log.field])
		}
	})
}

func TestUpDownAtTheContentStopMoveTheRowCursor(t *testing.T) {
	cases := []struct {
		name       string
		keys       []stroke
		wantCursor int
	}{
		{"down", []stroke{kt(tea.KeyDown)}, 1},
		{"j", typed("j"), 1},
		{"up", []stroke{kt(tea.KeyUp)}, 0},
		{"k", typed("k"), 0},
		{"down then up", []stroke{kt(tea.KeyDown), kt(tea.KeyUp)}, 0},
		{"up on the first row stays put", []stroke{kt(tea.KeyUp)}, 0},
		{"page down then page up", []stroke{kt(tea.KeyPgDown), kt(tea.KeyPgUp)}, 0},
		{"end then home", []stroke{kt(tea.KeyEnd), kt(tea.KeyHome)}, 0},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			a := startLog(t, rowOf("甲", "09:00", "10:00"), rowOf("乙", "11:00", "12:00"))

			send(t, a, c.keys...)

			if a.log.cursor != c.wantCursor {
				t.Errorf("cursor = %d, want %d", a.log.cursor, c.wantCursor)
			}
			// Moving about must never reorder or edit the day.
			wantRows(t, a,
				`"甲" start=09:00 end=10:00 dur=01h00m`,
				`"乙" start=11:00 end=12:00 dur=01h00m`)
		})
	}
}

// TestUpDownAtTheIndexStopMoveTheRowCursor covers the arrows on the stop that
// shows the number: a position is not a number to step, so ↑ and ↓ walk the list
// exactly as they do from the content stop, and the item itself stays put.
func TestUpDownAtTheIndexStopMoveTheRowCursor(t *testing.T) {
	untouched := []string{
		`"甲" start=09:00 end=10:00 dur=01h00m`,
		`"乙" start=11:00 end=12:00 dur=01h00m`,
	}

	cases := []struct {
		name       string
		cursor     int
		keys       []stroke
		wantCursor int
	}{
		{"down walks to the next row", 0, []stroke{kt(tea.KeyDown)}, 1},
		{"up walks back", 1, []stroke{kt(tea.KeyUp)}, 0},
		{"up on the first row stays put", 0, []stroke{kt(tea.KeyUp)}, 0},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			a := startLog(t, rowOf("甲", "09:00", "10:00"), rowOf("乙", "11:00", "12:00"))
			a.log.cursor = c.cursor
			park(t, a, fIndex)

			send(t, a, c.keys...)

			if a.log.cursor != c.wantCursor {
				t.Errorf("cursor = %d, want %d", a.log.cursor, c.wantCursor)
			}
			if a.log.field != fIndex {
				t.Errorf("cursor on %s, want it to stay on the index stop", stopNames[a.log.field])
			}
			wantRows(t, a, untouched...)
		})
	}
}

// TestJKCarryTheItemWhereverTheCursorStands covers the pair that moves the item
// itself. j and k only walk the cursor; J and K take the item the cursor is on
// down or up the list, and the reading the cursor happens to stand on has no say
// in it, so the pair works from every stop.
func TestJKCarryTheItemWhereverTheCursorStands(t *testing.T) {
	inOrder := []string{
		`"甲" start=09:00 end=10:00 dur=01h00m`,
		`"乙" start=11:00 end=12:00 dur=01h00m`,
		`"丙" start=13:00 end=14:00 dur=01h00m`,
	}
	movedDown := []string{inOrder[0], inOrder[2], inOrder[1]}
	movedUp := []string{inOrder[1], inOrder[0], inOrder[2]}

	cases := []struct {
		name       string
		cursor     int
		keys       []stroke
		wantRows   []string
		wantCursor int
	}{
		{"J takes the item to the next row", 1, typed("J"), movedDown, 2},
		{"K takes the item to the previous row", 1, typed("K"), movedUp, 0},
		{"J on the last row has nowhere to go", 2, typed("J"), inOrder, 2},
		{"K on the first row has nowhere to go", 0, typed("K"), inOrder, 0},
	}

	for _, c := range cases {
		for stop := 0; stop < stopCount; stop++ {
			t.Run(c.name+" on "+stopNames[stop], func(t *testing.T) {
				a := startLog(t,
					rowOf("甲", "09:00", "10:00"),
					rowOf("乙", "11:00", "12:00"),
					rowOf("丙", "13:00", "14:00"),
				)
				a.log.cursor = c.cursor
				park(t, a, stop)

				send(t, a, c.keys...)

				wantRows(t, a, c.wantRows...)
				if a.log.cursor != c.wantCursor {
					t.Errorf("cursor = %d, want %d: the cursor follows the item it moved", a.log.cursor, c.wantCursor)
				}
				if a.log.field != stop {
					t.Errorf("cursor on %s, want it to stay on %s", stopNames[a.log.field], stopNames[stop])
				}
			})
		}
	}
}

// TestJKLeaveTheReadingsAlone is the other half of the rule: carrying an item
// about moves the whole row, so the clocks travel with it and no number in the
// two rows involved is rewritten.
func TestJKLeaveTheReadingsAlone(t *testing.T) {
	a := startLog(t, rowOf("甲", "09:00", "10:00"), rowOf("乙", "11:00", "12:00"))
	a.log.cursor = 0
	park(t, a, fEndMinute)

	send(t, a, typed("J")...)

	wantRows(t, a,
		`"乙" start=11:00 end=12:00 dur=01h00m`,
		`"甲" start=09:00 end=10:00 dur=01h00m`)
	if a.log.cursor != 1 {
		t.Errorf("cursor = %d, want it to follow the item to row 2", a.log.cursor)
	}
}

// TestClockStopsStep pins what one press of an arrow does to the number under
// the cursor. Every case is one direction only, and the wrap cases use a press
// count whose answer the other key would not give, so swapping ↑ for ↓ cannot
// pass by landing where the inverse would.
func TestClockStopsStep(t *testing.T) {
	cases := []struct {
		name string
		stop int
		keys []stroke
		want []string
	}{
		{"up adds an hour", fStartHour, []stroke{kt(tea.KeyUp)},
			[]string{`"" start=10:00 end=12:00 dur=02h00m`}},
		{"down takes an hour off", fStartHour, []stroke{kt(tea.KeyDown)},
			[]string{`"" start=08:00 end=12:00 dur=04h00m`}},
		{"up adds five minutes", fStartMinute, []stroke{kt(tea.KeyUp)},
			[]string{`"" start=09:05 end=12:00 dur=02h55m`}},
		{"down takes five minutes off without borrowing the hour", fStartMinute, []stroke{kt(tea.KeyDown)},
			[]string{`"" start=09:55 end=12:00 dur=02h05m`}},
		{"hours climb round the clock", fStartHour, presses(tea.KeyUp, 14),
			[]string{`"" start=23:00 end=12:00 dur=13h00m`}},
		{"hours wrap past the top of the clock", fStartHour, presses(tea.KeyUp, 15),
			[]string{`"" start=00:00 end=12:00 dur=12h00m`}},
		{"hours wrap back past midnight", fStartHour, presses(tea.KeyDown, 10),
			[]string{`"" start=23:00 end=12:00 dur=13h00m`}},
		{"minutes climb round the hour", fStartMinute, presses(tea.KeyUp, 11),
			[]string{`"" start=09:55 end=12:00 dur=02h05m`}},
		{"minutes wrap past the top without carrying into the hour", fStartMinute, presses(tea.KeyUp, 13),
			[]string{`"" start=09:05 end=12:00 dur=02h55m`}},
		{"minutes wrap back past the bottom without borrowing the hour", fStartMinute, presses(tea.KeyDown, 13),
			[]string{`"" start=09:55 end=12:00 dur=02h05m`}},
		{"the end hour stop moves the end up", fEndHour, []stroke{kt(tea.KeyUp)},
			[]string{`"" start=09:00 end=13:00 dur=04h00m`}},
		{"the end hour stop moves the end down", fEndHour, []stroke{kt(tea.KeyDown)},
			[]string{`"" start=09:00 end=11:00 dur=02h00m`}},
		{"the end minute stop moves the end", fEndMinute, []stroke{kt(tea.KeyDown)},
			[]string{`"" start=09:00 end=12:55 dur=03h55m`}},
		{"the end minute stop moves the end up", fEndMinute, []stroke{kt(tea.KeyUp)},
			[]string{`"" start=09:00 end=12:05 dur=03h05m`}},
		{"an end run back behind the start reads as the next day", fEndHour, presses(tea.KeyDown, 13),
			[]string{`"" start=09:00 end=23:00 dur=14h00m`}},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			a := startLog(t, rowOf("", "09:00", "12:00"))
			park(t, a, c.stop)

			send(t, a, c.keys...)

			wantRows(t, a, c.want...)
			wantFile(t, a)
			if a.log.field != c.stop {
				t.Errorf("cursor on %s, want it to stay on %s", stopNames[a.log.field], stopNames[c.stop])
			}
		})
	}
}

func TestEmptyClockStopsStartAtMidnight(t *testing.T) {
	cases := []struct {
		name string
		stop int
		keys []stroke
		want []string
	}{
		{"an empty start hour is built at midnight and stepped", fStartHour, []stroke{kt(tea.KeyUp)},
			[]string{`"" start=01:00 end=unset dur=--`}},
		{"an empty start minute is stepped by five", fStartMinute, []stroke{kt(tea.KeyUp)},
			[]string{`"" start=00:05 end=unset dur=--`}},
		{"an empty end hour is created as well", fEndHour, []stroke{kt(tea.KeyUp)},
			[]string{`"" start=unset end=01:00 dur=--`}},
		{"stepping down from nothing wraps backwards", fEndMinute, []stroke{kt(tea.KeyDown)},
			[]string{`"" start=unset end=00:55 dur=--`}},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			a := startLog(t, rowOf("", "", ""))
			park(t, a, c.stop)

			send(t, a, c.keys...)

			wantRows(t, a, c.want...)
		})
	}
}

// TestDurationStopsStepTheSpan covers the two stops that own no number of their
// own: they write through the end time, so every case names the end it expects
// as well as the span. As on the clock stops, one press per direction.
func TestDurationStopsStepTheSpan(t *testing.T) {
	cases := []struct {
		name       string
		start, end string
		stop       int
		keys       []stroke
		want       []string
	}{
		{"up adds an hour on the span", "09:00", "09:00", fDurHour, []stroke{kt(tea.KeyUp)},
			[]string{`"" start=09:00 end=10:00 dur=01h00m`}},
		{"down takes an hour off the span", "09:00", "10:00", fDurHour, []stroke{kt(tea.KeyDown)},
			[]string{`"" start=09:00 end=09:00 dur=00h00m`}},
		{"five minutes on the span", "09:00", "09:00", fDurMinute, []stroke{kt(tea.KeyUp)},
			[]string{`"" start=09:00 end=09:05 dur=00h05m`}},
		{"five minutes off the span", "09:00", "09:05", fDurMinute, []stroke{kt(tea.KeyDown)},
			[]string{`"" start=09:00 end=09:00 dur=00h00m`}},
		{"taking hours off wraps the other way", "09:00", "10:00", fDurHour, presses(tea.KeyDown, 2),
			[]string{`"" start=09:00 end=08:00 dur=23h00m`}},
		{"minutes wrap without carrying into the hour", "09:00", "09:00", fDurMinute, presses(tea.KeyUp, 13),
			[]string{`"" start=09:00 end=09:05 dur=00h05m`}},
		{"minutes wrap back without borrowing the hour", "09:00", "09:05", fDurMinute, presses(tea.KeyDown, 14),
			[]string{`"" start=09:00 end=09:55 dur=00h55m`}},
		{"an item with no end starts from nothing", "09:00", "", fDurHour, []stroke{kt(tea.KeyUp)},
			[]string{`"" start=09:00 end=10:00 dur=01h00m`}},
		{"the start is never moved", "09:00", "10:00", fDurHour, presses(tea.KeyUp, 3),
			[]string{`"" start=09:00 end=13:00 dur=04h00m`}},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			a := startLog(t, rowOf("", c.start, c.end))
			park(t, a, c.stop)

			send(t, a, c.keys...)

			wantRows(t, a, c.want...)
			wantFile(t, a)
		})
	}
}

// TestDurationWithoutAStartIsLeftAlone covers the span stops on a row with
// nothing to measure from: neither a step nor a typed digit may invent an end
// time, each says why, and neither keeps the turn it was offered.
func TestDurationWithoutAStartIsLeftAlone(t *testing.T) {
	cases := []struct {
		name string
		stop int
		keys []stroke
	}{
		{"stepping the hours up", fDurHour, []stroke{kt(tea.KeyUp)}},
		{"stepping the hours down", fDurHour, []stroke{kt(tea.KeyDown)}},
		{"stepping the minutes up", fDurMinute, []stroke{kt(tea.KeyUp)}},
		{"stepping the minutes down", fDurMinute, []stroke{kt(tea.KeyDown)}},
		{"the tens of a span of hours", fDurHour, typed("2")},
		{"both halves of a span of hours", fDurHour, typed("20")},
		{"the tens of a span of minutes", fDurMinute, typed("4")},
		{"both halves of a span of minutes", fDurMinute, typed("45")},
		{"a lone digit, which no timer will ever commit", fDurMinute, typed("5")},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			a := startLog(t, rowOf("未开始", "", ""))
			park(t, a, c.stop)

			send(t, a, c.keys...)
			if cmd := a.onTick(time.Now().Add(time.Hour)); cmd != nil {
				t.Error("an hour later a tick was still owed, want digit entry to own no timer")
			}

			wantRows(t, a, `"未开始" start=unset end=unset dur=--`)
			if a.status != "耗时由起止时间算出，先填开始时间" {
				t.Errorf("status = %q, want the warning that the span comes from the clock", a.status)
			}
			if a.log.field != c.stop {
				t.Errorf("cursor on %s, want it to stay put", stopNames[a.log.field])
			}
			if !a.log.tensNext {
				t.Error("turn = the ones, want a digit the reading refused to keep the turn")
			}
			if _, err := os.Stat(a.store.Path); !os.IsNotExist(err) {
				t.Errorf("a refused span reached the file (Stat err = %v), want nothing written", err)
			}
		})
	}
}

func TestEscAndEnterLeaveAnyStop(t *testing.T) {
	strokes := []struct {
		name string
		msg  stroke
	}{{"Esc", kt(tea.KeyEsc)}, {"Enter", kt(tea.KeyEnter)}}

	for stop := fIndex; stop < fContent; stop++ {
		for _, k := range strokes {
			t.Run(stopNames[stop]+" "+k.name, func(t *testing.T) {
				a := startLog(t, rowOf("甲", "09:00", "10:00"))
				park(t, a, stop)

				cmd := send(t, a, k.msg)

				if a.log.field != fContent {
					t.Errorf("cursor on %s, want the content stop", stopNames[a.log.field])
				}
				if a.log.editing {
					t.Error("editing = true, want Enter to leave the editor closed")
				}
				if isQuit(t, cmd) {
					t.Error("the key quit the program")
				}
				wantRows(t, a, `"甲" start=09:00 end=10:00 dur=01h00m`)
			})
		}
	}
}

func TestSFillsAClockStopWithNow(t *testing.T) {
	cases := []struct {
		name      string
		stop      int
		fills     string // which bound the key writes
		untouched string // and which one it must leave alone
	}{
		{"start hour", fStartHour, "start", "10:00"},
		{"start minute", fStartMinute, "start", "10:00"},
		{"end hour", fEndHour, "end", "09:00"},
		{"end minute", fEndMinute, "end", "09:00"},
	}

	tsMinutes := func(x store.Timestamp) int { return x.Hour*60 + x.Minute }

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			a := startLog(t, rowOf("甲", "09:00", "10:00"))
			park(t, a, c.stop)

			before := store.NowTimestamp()
			send(t, a, ch('s'))
			after := store.NowTimestamp()

			it := a.items()[0]
			got, other := it.End, it.Start
			if c.fills == "start" {
				got, other = it.Start, it.End
			}
			if got == nil {
				t.Fatal("the reading is still unset")
			}
			if m := tsMinutes(*got); m < tsMinutes(before) || m > tsMinutes(after) {
				t.Errorf("filled %s, want the clock between %02d:%02d and %02d:%02d", timeText(got), before.Hour, before.Minute, after.Hour, after.Minute)
			}
			if g := timeText(other); g != c.untouched {
				t.Errorf("the other reading = %s, want it left at %s", g, c.untouched)
			}
			if it.Content != "甲" {
				t.Errorf("content = %q, want the row's own text", it.Content)
			}
			if a.log.field != c.stop {
				t.Errorf("cursor on %s, want it to stay on %s", stopNames[a.log.field], stopNames[c.stop])
			}
		})
	}
}

func TestSDoesNothingOnTheOtherStops(t *testing.T) {
	for _, stop := range []int{fIndex, fDurHour, fDurMinute, fContent} {
		t.Run(stopNames[stop], func(t *testing.T) {
			a := startLog(t, rowOf("甲", "09:00", "10:00"))
			park(t, a, stop)

			send(t, a, ch('s'))

			wantRows(t, a, `"甲" start=09:00 end=10:00 dur=01h00m`)
			if a.log.field != stop {
				t.Errorf("cursor on %s, want it to stay on %s", stopNames[a.log.field], stopNames[stop])
			}
			if a.log.editing || a.log.cursor != 0 {
				t.Errorf("editing = %v cursor = %d, want the key inert", a.log.editing, a.log.cursor)
			}
			if _, err := os.Stat(a.store.Path); !os.IsNotExist(err) {
				t.Errorf("an inert key wrote the file (Stat err = %v), want nothing saved", err)
			}
		})
	}
}

func TestXClearsAClockStop(t *testing.T) {
	cases := []struct {
		name   string
		stop   int
		clears string // which bound the key clears
		kept   string // and which one it must leave alone
	}{
		{"start hour", fStartHour, "start", "10:00"},
		{"start minute", fStartMinute, "start", "10:00"},
		{"end hour", fEndHour, "end", "09:00"},
		{"end minute", fEndMinute, "end", "09:00"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			a := startLog(t, rowOf("甲", "09:00", "10:00"))
			park(t, a, c.stop)

			send(t, a, ch('x'))

			it := a.items()[0]
			got, other := it.End, it.Start
			if c.clears == "start" {
				got, other = it.Start, it.End
			}
			if got != nil {
				t.Errorf("the %s reading is still %s, want it cleared", c.clears, got)
			}
			if g := timeText(other); g != c.kept {
				t.Errorf("the other reading = %s, want it left at %s", g, c.kept)
			}
			if a.log.field != c.stop {
				t.Errorf("cursor on %s, want it to stay on %s", stopNames[a.log.field], stopNames[c.stop])
			}
		})
	}
}

func TestXDoesNothingOnTheOtherStops(t *testing.T) {
	for _, stop := range []int{fIndex, fTodo, fDurHour, fDurMinute, fContent} {
		t.Run(stopNames[stop], func(t *testing.T) {
			a := startLog(t, rowOf("甲", "09:00", "10:00"))
			park(t, a, stop)

			send(t, a, ch('x'))

			wantRows(t, a, `"甲" start=09:00 end=10:00 dur=01h00m`)
			if a.log.field != stop {
				t.Errorf("cursor on %s, want it to stay on %s", stopNames[a.log.field], stopNames[stop])
			}
			if a.log.editing || a.log.cursor != 0 {
				t.Errorf("editing = %v cursor = %d, want the key inert", a.log.editing, a.log.cursor)
			}
		})
	}
}

// TestDigitsFillAClockStop types a pair of numerals into each measured stop and
// checks the reading they leave behind. The order the two halves are filled in
// is the subject of the sequence tests that follow.
func TestDigitsFillAClockStop(t *testing.T) {
	cases := []struct {
		name       string
		start, end string
		stop       int
		keys       string
		want       []string
	}{
		{"two digits for an hour", "09:00", "12:00", fStartHour, "14",
			[]string{`"" start=14:00 end=12:00 dur=22h00m`}},
		{"two digits for a minute", "09:00", "12:00", fStartMinute, "45",
			[]string{`"" start=09:45 end=12:00 dur=02h15m`}},
		{"a second digit takes the zero", "09:00", "12:00", fStartMinute, "10",
			[]string{`"" start=09:10 end=12:00 dur=02h50m`}},
		{"the end hour", "09:00", "12:00", fEndHour, "18",
			[]string{`"" start=09:00 end=18:00 dur=09h00m`}},
		{"the end minute", "09:00", "12:00", fEndMinute, "05",
			[]string{`"" start=09:00 end=12:05 dur=03h05m`}},
		{"a digit pair lands on an empty reading", "", "12:00", fStartMinute, "30",
			[]string{`"" start=00:30 end=12:00 dur=11h30m`}},
		{"an empty hour starts at the tens too", "", "", fStartHour, "20",
			[]string{`"" start=20:00 end=unset dur=--`}},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			a := startLog(t, rowOf("", c.start, c.end))
			park(t, a, c.stop)

			send(t, a, typed(c.keys)...)

			wantRows(t, a, c.want...)
			if a.status != "" {
				t.Errorf("status = %q, want a silent keystroke", a.status)
			}
			if a.log.field != c.stop {
				t.Errorf("cursor on %s, want it to stay on %s", stopNames[a.log.field], stopNames[c.stop])
			}
			wantFile(t, a)
		})
	}

	t.Run("a leading zero belongs to the reading", func(t *testing.T) {
		a := startLog(t, rowOf("", "15:00", "12:00"))
		park(t, a, fStartHour)

		send(t, a, typed("09")...)

		// 0 picks the index stop everywhere else, but 09:00 would be untypeable
		// if it did that here too.
		wantRows(t, a, `"" start=09:00 end=12:00 dur=03h00m`)
		if a.log.field != fStartHour {
			t.Errorf("cursor on %s, want it to stay on the hour", stopNames[a.log.field])
		}
	})
}

// digitStep is one press of a typed sequence and everything the page has to show
// once it lands: the row, the status line — empty for a silent key — and whose
// turn the next press is.
type digitStep struct {
	key    rune
	want   string
	status string
	tens   bool
}

// pressDigits parks on a stop and works through a sequence one key at a time,
// checking the whole page after every press rather than only the answer at the
// end. An accepted press has to have reached the file; a refused one may not
// even touch it.
func pressDigits(t *testing.T, start, end string, stop int, steps ...digitStep) {
	t.Helper()

	a := startLog(t, rowOf("甲", start, end))
	park(t, a, stop)

	for _, s := range steps {
		before := fileText(t, a)
		send(t, a, ch(s.key))

		if got := rowState(a, 0); got != s.want {
			t.Fatalf("after %c: row = %s, want %s", s.key, got, s.want)
		}
		if a.status != s.status {
			t.Fatalf("after %c: status = %q, want %q", s.key, a.status, s.status)
		}
		if a.log.field != stop {
			t.Fatalf("after %c: cursor on %s, want it to stay on %s",
				s.key, stopNames[a.log.field], stopNames[stop])
		}
		if a.log.tensNext != s.tens {
			t.Errorf("after %c: the next digit is the %s, want the %s",
				s.key, halfName(a.log.tensNext), halfName(s.tens))
		}
		if s.status == "" {
			wantFile(t, a)
		} else if got := fileText(t, a); got != before {
			t.Errorf("after %c: a refused digit rewrote the file", s.key)
		}
	}
}

func halfName(tens bool) string {
	if tens {
		return "tens"
	}
	return "ones"
}

// TestThePinnedHourSequenceFillsTheTensThenTheOnes is the sequence the design
// was agreed on: off an hour of 00, a 1 gives 10, a 2 gives 12, a 0 gives 02 and
// a 5 gives 05. Each press overwrites one half of the reading and hands the turn
// to the other.
func TestThePinnedHourSequenceFillsTheTensThenTheOnes(t *testing.T) {
	pressDigits(t, "00:00", "", fStartHour,
		digitStep{'1', `"甲" start=10:00 end=unset dur=--`, "", false},
		digitStep{'2', `"甲" start=12:00 end=unset dur=--`, "", true},
		digitStep{'0', `"甲" start=02:00 end=unset dur=--`, "", false},
		digitStep{'5', `"甲" start=05:00 end=unset dur=--`, "", true},
	)
}

// TestThePinnedHourSequenceRefusesWhatTheClockHasNoRoomFor is the second agreed
// sequence. The 3 and the 2 both offer the tens an hour past the top of the
// clock, and both are dropped without passing the turn — which is why the 1 that
// follows is still a tens digit, and the 8 after it lands on the ones.
func TestThePinnedHourSequenceRefusesWhatTheClockHasNoRoomFor(t *testing.T) {
	pressDigits(t, "00:00", "", fStartHour,
		digitStep{'1', `"甲" start=10:00 end=unset dur=--`, "", false},
		digitStep{'9', `"甲" start=19:00 end=unset dur=--`, "", true},
		digitStep{'3', `"甲" start=19:00 end=unset dur=--`, "开始 小时 最大 23", true},
		digitStep{'2', `"甲" start=19:00 end=unset dur=--`, "开始 小时 最大 23", true},
		digitStep{'1', `"甲" start=19:00 end=unset dur=--`, "", false},
		digitStep{'8', `"甲" start=18:00 end=unset dur=--`, "", true},
	)
}

// TestThePinnedMinuteSequenceFillsAndRefusesInTurn is the same walk on a minute,
// where the ceiling is 59 and the tens of 09 is still the tens of 09.
func TestThePinnedMinuteSequenceFillsAndRefusesInTurn(t *testing.T) {
	pressDigits(t, "09:00", "", fStartMinute,
		digitStep{'6', `"甲" start=09:00 end=unset dur=--`, "开始 分钟 最大 59", true},
		digitStep{'5', `"甲" start=09:50 end=unset dur=--`, "", false},
		digitStep{'9', `"甲" start=09:59 end=unset dur=--`, "", true},
		digitStep{'6', `"甲" start=09:59 end=unset dur=--`, "开始 分钟 最大 59", true},
		digitStep{'2', `"甲" start=09:29 end=unset dur=--`, "", false},
		digitStep{'5', `"甲" start=09:25 end=unset dur=--`, "", true},
	)
}

// TestAnOnesDigitRefusedKeepsTheOnesTurn covers the other half: a ones digit the
// clock has no room for is dropped too, so the next press can finish the reading.
func TestAnOnesDigitRefusedKeepsTheOnesTurn(t *testing.T) {
	pressDigits(t, "00:00", "", fStartHour,
		digitStep{'2', `"甲" start=20:00 end=unset dur=--`, "", false},
		digitStep{'5', `"甲" start=20:00 end=unset dur=--`, "开始 小时 最大 23", false},
		digitStep{'3', `"甲" start=23:00 end=unset dur=--`, "", true},
	)
}

// TestDigitsFillTheSpanThroughTheEndTime types into the two stops that own no
// number of their own. The span lands on the end time, and a pair the clock has
// no room for is refused rather than clamped.
func TestDigitsFillTheSpanThroughTheEndTime(t *testing.T) {
	cases := []struct {
		name string
		stop int
		keys string
		want []string
	}{
		{"two hours", fDurHour, "11", []string{`"" start=09:00 end=20:00 dur=11h00m`}},
		{"minutes replace only the minute half", fDurMinute, "45",
			[]string{`"" start=09:00 end=10:45 dur=01h45m`}},
		{"hours the clock has no room for leave the span alone", fDurHour, "99",
			[]string{`"" start=09:00 end=10:00 dur=01h00m`}},
		{"minutes the clock has no room for leave the span alone", fDurMinute, "99",
			[]string{`"" start=09:00 end=10:00 dur=01h00m`}},
		{"a refused tens leaves the turn for the next press", fDurHour, "905",
			[]string{`"" start=09:00 end=14:00 dur=05h00m`}},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			a := startLog(t, rowOf("", "09:00", "10:00"))
			park(t, a, c.stop)

			send(t, a, typed(c.keys)...)

			wantRows(t, a, c.want...)
		})
	}
}

// TestATypedDigitIsTakenImmediatelyAndShownInItsOwnCell is the other half of
// retiring the timed entry: the first press already writes the reading, the cell
// shows it whole, the hint bar echoes nothing, and no tick is owed.
func TestATypedDigitIsTakenImmediatelyAndShownInItsOwnCell(t *testing.T) {
	a := startLog(t, rowOf("甲", "00:00", "12:00"))
	park(t, a, fStartHour)

	started := time.Now()
	if cmd := send(t, a, ch('1')); cmd != nil {
		t.Errorf("a typed digit scheduled %v, want nothing to wait for", cmd)
	}

	wantRows(t, a, `"甲" start=10:00 end=12:00 dur=02h00m`)
	if plain := ansi.Strip(a.viewLog()); !strings.Contains(plain, "10:00") {
		t.Errorf("frame does not show the hour the digit wrote:\n%s", plain)
	}
	if plain := ansi.Strip(a.viewLog()); strings.Contains(plain, "1_") {
		t.Errorf("frame still shows a half-typed digit:\n%s", plain)
	}
	if hints := ansi.Strip(a.renderStatus()); strings.HasPrefix(hints, "[") {
		t.Errorf("hint bar %q echoes a digit the reading has already taken", hints)
	}
	if cmd := a.onTick(started.Add(time.Hour)); cmd != nil {
		t.Error("an hour later a tick was still owed, want digit entry to own no timer")
	}
	wantFile(t, a)
}

// TestArrivingAtAStopStartsAtTheTens covers that the turn belongs to the stop,
// not to the key sequence: every way of landing on a stop restarts it.
func TestArrivingAtAStopStartsAtTheTens(t *testing.T) {
	t.Run("walking to the next stop", func(t *testing.T) {
		a := startLog(t, rowOf("甲", "00:00", "12:00"))
		park(t, a, fStartHour)

		send(t, a, ch('1')) // the hour's tens: 10:00, the ones is next
		if a.log.tensNext {
			t.Fatal("turn = the tens, want an accepted tens digit to pass it to the ones")
		}

		send(t, a, kt(tea.KeyTab), ch('1')) // the minute stop, from its tens
		wantRows(t, a, `"甲" start=10:10 end=12:00 dur=01h50m`)
	})

	t.Run("walking back to a half-typed stop", func(t *testing.T) {
		a := startLog(t, rowOf("甲", "00:00", "12:00"))
		park(t, a, fStartHour)

		send(t, a, ch('1'), kt(tea.KeyTab), kt(tea.KeyShiftTab), ch('2'))
		// The hour stopped at 10 with the ones next, but standing on it again
		// restarts at the tens, so the 2 is a ten.
		wantRows(t, a, `"甲" start=20:00 end=12:00 dur=16h00m`)
	})

	t.Run("walking off keeps what was typed", func(t *testing.T) {
		a := startLog(t, rowOf("甲", "00:00", "12:00"))
		park(t, a, fStartHour)

		send(t, a, typed("07")...)
		send(t, a, kt(tea.KeyTab))

		wantRows(t, a, `"甲" start=07:00 end=12:00 dur=05h00m`)
		if a.log.field != fStartMinute {
			t.Errorf("cursor on %s, want the stop Tab steps to", stopNames[a.log.field])
		}
		wantFile(t, a)
	})

	t.Run("every key that lands somewhere restarts the turn", func(t *testing.T) {
		cases := []struct {
			name string
			keys []stroke
		}{
			{"Esc back to the content", []stroke{kt(tea.KeyEsc)}},
			{"Enter back to the content", []stroke{kt(tea.KeyEnter)}},
			{"the clock key on this stop", typed("s")},
			{"a step forward", []stroke{kt(tea.KeyTab)}},
			{"a step back", []stroke{kt(tea.KeyShiftTab)}},
			{"opening the editor", typed("i")},
			{"appending to the content", typed("a")},
		}

		for _, c := range cases {
			t.Run(c.name, func(t *testing.T) {
				a := startLog(t, rowOf("甲", "00:00", "12:00"))
				park(t, a, fStartHour)

				send(t, a, ch('1'))
				if a.log.tensNext {
					t.Fatal("turn = the tens after the tens, want the ones next")
				}

				send(t, a, c.keys...)

				if !a.log.tensNext {
					t.Errorf("turn = the ones after %v, want landing to restart at the tens", c.keys)
				}
			})
		}
	})

	t.Run("jumping to another row", func(t *testing.T) {
		a := startLog(t, rowOf("甲", "00:00", "12:00"), rowOf("乙", "00:00", "12:00"))
		park(t, a, fStartHour)

		send(t, a, ch('1'))
		if a.log.tensNext {
			t.Fatal("turn = the tens mid-fill, want the ones next")
		}

		// The half-typed hour belongs to 甲; 乙 is a fresh row, so it starts at
		// the tens.
		send(t, a, ch('G'), ch('2'))
		wantRows(t, a,
			`"甲" start=10:00 end=12:00 dur=02h00m`,
			`"乙" start=20:00 end=12:00 dur=16h00m`,
		)
	})
}

// TestDigitsOffTheClockStopsNeverTouchAReading is the other side of the rule: on
// the index and the content stop a digit is the jump machine's, so the readings
// keep the clock they were given however many are typed. Which row a digit run
// lands on is TestDigitsStillJumpToAnItem's business.
func TestDigitsOffTheClockStopsNeverTouchAReading(t *testing.T) {
	cases := []struct {
		name       string
		stop       int
		keys       string
		wantCursor int
	}{
		{"a digit on the index stop", fIndex, "2", 1},
		{"a digit on the content stop", fContent, "2", 1},
		{"a long run on the index stop", fIndex, "1830", 11},
		{"a long run on the content stop", fContent, "1830", 11},
	}

	var many []store.Item
	for i := range 12 {
		many = append(many, rowOf(fmt.Sprintf("项%d", i+1), "09:00", "09:30"))
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			a := startLog(t, many...)
			park(t, a, c.stop)

			send(t, a, typed(c.keys)...)

			for i := range a.items() {
				if got := rowState(a, i); got != fmt.Sprintf(`"项%d" start=09:00 end=09:30 dur=00h30m`, i+1) {
					t.Errorf("row %d = %s, want a jump to leave every reading alone", i, got)
				}
			}
			if a.log.cursor != c.wantCursor {
				t.Errorf("cursor = %d, want %d: the digit belongs to the jump machine",
					a.log.cursor, c.wantCursor)
			}
			if a.log.field != c.stop {
				t.Errorf("cursor on %s, want it to stay on %s", stopNames[a.log.field], stopNames[c.stop])
			}
		})
	}
}

// TestDigitsOnAMeasuredStopNeverReachTheJumpMachine is the same keys on the six
// stops that own a reading: the number belongs to the cell, not to the list.
func TestDigitsOnAMeasuredStopNeverReachTheJumpMachine(t *testing.T) {
	want := map[int]string{
		fStartHour:   `"项4" start=20:00 end=00:00 dur=04h00m`,
		fStartMinute: `"项4" start=00:20 end=00:00 dur=23h40m`,
		fEndHour:     `"项4" start=00:00 end=20:00 dur=20h00m`,
		fEndMinute:   `"项4" start=00:00 end=00:20 dur=00h20m`,
		fDurHour:     `"项4" start=00:00 end=20:00 dur=20h00m`,
		fDurMinute:   `"项4" start=00:00 end=00:20 dur=00h20m`,
	}

	for stop := fStartHour; stop < fContent; stop++ {
		t.Run(stopNames[stop], func(t *testing.T) {
			// A row's clocks are pointers, so the rows are built fresh for every
			// subtest: one shared slice would let each case type into the last.
			var many []store.Item
			for i := range 5 {
				many = append(many, rowOf(fmt.Sprintf("项%d", i+1), "00:00", "00:00"))
			}

			a := startLog(t, many...)
			a.log.cursor = 3
			park(t, a, stop)

			send(t, a, ch('2'))

			if a.log.cursor != 3 {
				t.Errorf("cursor = %d, want the digit to be a reading, not a jump to item 2", a.log.cursor)
			}
			if a.log.field != stop {
				t.Errorf("cursor on %s, want it to stay on %s", stopNames[a.log.field], stopNames[stop])
			}
			if got := rowState(a, 3); got != want[stop] {
				t.Errorf("row 3 = %s, want %s", got, want[stop])
			}
			if got := rowState(a, 0); got != `"项1" start=00:00 end=00:00 dur=00h00m` {
				t.Errorf("row 0 = %s, want only the focused row typed into", got)
			}
			if hints := ansi.Strip(a.renderStatus()); strings.HasPrefix(hints, "[") {
				t.Errorf("hint bar %q echoes a digit the reading has taken", hints)
			}
		})
	}
}

func TestEditorOpensFromAnyStopAndCommits(t *testing.T) {
	for _, stop := range []int{fIndex, fStartHour, fDurMinute, fContent} {
		for _, opener := range []struct {
			key  rune
			want string
		}{{'i', "新旧内容"}, {'a', "旧内容新"}} {
			t.Run(stopNames[stop]+" "+string(opener.key), func(t *testing.T) {
				a := startLog(t, rowOf("旧内容", "09:00", "10:00"))
				park(t, a, stop)

				send(t, a, ch(opener.key))

				if !a.log.editing {
					t.Fatal("editing = false, want the editor open")
				}
				if a.log.field != fContent {
					t.Errorf("cursor on %s, want the content stop while editing", stopNames[a.log.field])
				}
				if got := a.log.editor.Value(); got != "旧内容" {
					t.Errorf("editor holds %q, want the row's own text", got)
				}

				send(t, a, typed("新")...)
				send(t, a, kt(tea.KeyEnter))

				if a.log.editing {
					t.Error("editing = true after Enter")
				}
				wantRows(t, a, fmt.Sprintf("%q start=09:00 end=10:00 dur=01h00m", opener.want))
			})
		}
	}
}

func TestEscInEditorAlsoCommits(t *testing.T) {
	a := startLog(t, rowOf("甲", "09:00", "10:00"))

	send(t, a, ch('a'), ch('!'))
	send(t, a, kt(tea.KeyEsc))

	if a.log.editing {
		t.Error("editing = true after Esc")
	}
	wantRows(t, a, `"甲!" start=09:00 end=10:00 dur=01h00m`)
}

func TestRowCommandsDoNotFireWhileEditing(t *testing.T) {
	a := startLog(t, rowOf("甲", "09:00", "10:00"), rowOf("乙", "11:00", "12:00"))

	send(t, a, ch('a'))
	cmd := send(t, a, typed("qjkdh")...)

	if isQuit(t, cmd) {
		t.Fatal("q quit the program from inside the editor")
	}
	if len(a.items()) != 2 {
		t.Fatalf("%d rows, want both", len(a.items()))
	}
	if a.log.cursor != 0 {
		t.Errorf("cursor = %d, want the keys to be text, not movement", a.log.cursor)
	}
	if !a.log.editing || a.log.field != fContent {
		t.Errorf("cursor on %s editing=%v, want the editor still open", stopNames[a.log.field], a.log.editing)
	}
	if got := a.log.editor.Value(); got != "甲qjkdh" {
		t.Errorf("editor holds %q, want every key typed into it", got)
	}
}

func TestOpenInsertsBelowAndStartsTheClock(t *testing.T) {
	a := startLog(t, rowOf("甲", "09:00", "10:00"), rowOf("乙", "11:00", "12:00"))
	a.log.cursor = 0

	before := store.NowTimestamp()
	send(t, a, ch('o'))
	after := store.NowTimestamp()

	if !a.log.editing || a.log.field != fContent {
		t.Errorf("cursor on %s editing=%v, want the editor open on the new row",
			stopNames[a.log.field], a.log.editing)
	}
	if a.log.cursor != 1 {
		t.Fatalf("cursor = %d, want the new row below the one the cursor was on", a.log.cursor)
	}
	if len(a.items()) != 3 {
		t.Fatalf("%d rows, want one more than the two we started with", len(a.items()))
	}
	if got := rowState(a, 0); got != `"甲" start=09:00 end=10:00 dur=01h00m` {
		t.Errorf("row 0 = %s, want it left alone", got)
	}
	if got := rowState(a, 2); got != `"乙" start=11:00 end=12:00 dur=01h00m` {
		t.Errorf("row 2 = %s, want the shifted row left alone", got)
	}

	fresh := a.items()[1]
	if fresh.Content != "" {
		t.Errorf("new row content = %q, want it empty for the editor to fill", fresh.Content)
	}
	if fresh.End != nil {
		t.Errorf("new row ends at %s, want no end time yet", fresh.End)
	}
	start := fresh.Start
	if start == nil {
		t.Fatal("the new row has no start time")
	}
	tsMinutes := func(x store.Timestamp) int { return x.Hour*60 + x.Minute }
	if m := tsMinutes(*start); m < tsMinutes(before) || m > tsMinutes(after) {
		t.Errorf("new row starts at %s, want the clock between %02d:%02d and %02d:%02d", timeText(start), before.Hour, before.Minute, after.Hour, after.Minute)
	}

	send(t, a, typed("新事项")...)
	send(t, a, kt(tea.KeyEnter))

	wantRows(t, a,
		`"甲" start=09:00 end=10:00 dur=01h00m`,
		fmt.Sprintf(`"新事项" start=%s end=unset dur=--`, timeText(start)),
		`"乙" start=11:00 end=12:00 dur=01h00m`)
}

// TestOpenClosesTheOpenItemAboveIt covers the other half of o: the new row starts
// now, so the row above it — the task logged until this moment — stops at that
// same reading, but only if it never got an end of its own. A row closed already,
// or never started, is left as it is, and the two readings are separate values,
// so editing one of them cannot move the other.
func TestOpenClosesTheOpenItemAboveIt(t *testing.T) {
	t.Run("an open row above stops where the new one starts", func(t *testing.T) {
		a := startLog(t, rowOf("还没收尾", "09:00", ""))

		send(t, a, ch('o'))

		if len(a.items()) != 2 {
			t.Fatalf("%d rows, want the new one below", len(a.items()))
		}
		prev, fresh := a.items()[0], a.items()[1]
		if prev.End == nil || fresh.Start == nil {
			t.Fatalf("prev end=%v fresh start=%v, want the handover written", prev.End, fresh.Start)
		}
		if *prev.End != *fresh.Start {
			t.Errorf("the row above stops at %s but the new one starts at %s, want one handover",
				prev.End, fresh.Start)
		}
		if prev.End == fresh.Start {
			t.Error("the two readings share one pointer: editing either would move the other")
		}
	})

	t.Run("a row closed already keeps its own end", func(t *testing.T) {
		a := startLog(t, rowOf("已经收尾", "09:00", "10:00"))

		send(t, a, ch('o'))

		if got := rowState(a, 0); got != `"已经收尾" start=09:00 end=10:00 dur=01h00m` {
			t.Errorf("row 0 = %s, want its own end kept", got)
		}
	})

	t.Run("a row that never started has nothing to close", func(t *testing.T) {
		a := startLog(t, rowOf("还没开始", "", ""))

		send(t, a, ch('o'))

		if a.items()[0].End != nil {
			t.Errorf("row 0 ends at %s, want a row with no start left open", a.items()[0].End)
		}
	})

	t.Run("an empty day has nothing above the new row", func(t *testing.T) {
		a := startLog(t)

		send(t, a, ch('o'))

		if len(a.items()) != 1 || a.items()[0].End != nil {
			t.Errorf("%d rows ending at %v, want one open row", len(a.items()), a.items()[0].End)
		}
	})
}

// TestOpenFromAStopInsertsBelow covers that o is about the row wherever the
// cursor stands: the new row starts now, has no end yet, and opens the editor on
// the content stop with digit entry restarted at the tens.
func TestOpenFromAStopInsertsBelow(t *testing.T) {
	for _, stop := range []int{fIndex, fStartHour, fDurMinute} {
		t.Run(stopNames[stop], func(t *testing.T) {
			a := startLog(t, rowOf("甲", "09:00", "10:00"))
			a.log.cursor = 0
			park(t, a, stop)

			before := store.NowTimestamp()
			send(t, a, ch('o'))
			after := store.NowTimestamp()

			if len(a.items()) != 2 {
				t.Fatalf("%d rows, want the one new row below", len(a.items()))
			}
			if !a.log.editing || a.log.field != fContent {
				t.Errorf("editing = %v cursor on %s, want the editor open on the content stop",
					a.log.editing, stopNames[a.log.field])
			}
			if a.log.cursor != 1 {
				t.Errorf("cursor = %d, want the new row", a.log.cursor)
			}
			if !a.log.tensNext {
				t.Error("turn = the ones, want landing on a stop through the editor to restart at the tens")
			}

			fresh := a.items()[1]
			if fresh.End != nil {
				t.Errorf("new row ends at %s, want no end time yet", fresh.End)
			}
			if fresh.Start == nil {
				t.Fatal("the new row has no start time")
			}
			tsMinutes := func(x store.Timestamp) int { return x.Hour*60 + x.Minute }
			if m := tsMinutes(*fresh.Start); m < tsMinutes(before) || m > tsMinutes(after) {
				t.Errorf("new row starts at %s, want the clock between %02d:%02d and %02d:%02d", timeText(fresh.Start), before.Hour, before.Minute, after.Hour, after.Minute)
			}

			send(t, a, kt(tea.KeyEnter))

			wantRows(t, a,
				`"甲" start=09:00 end=10:00 dur=01h00m`,
				fmt.Sprintf(`"" start=%s end=unset dur=--`, timeText(fresh.Start)))
			wantFile(t, a)
		})
	}
}

func TestOpenAtTheEndOfTheList(t *testing.T) {
	t.Run("last row of a full day", func(t *testing.T) {
		a := startLog(t, rowOf("甲", "09:00", "10:00"), rowOf("乙", "11:00", "12:00"))
		a.log.cursor = 1

		send(t, a, ch('o'), kt(tea.KeyEnter))

		if a.log.cursor != 2 {
			t.Errorf("cursor = %d, want the new last row", a.log.cursor)
		}
		if len(a.items()) != 3 {
			t.Fatalf("%d rows, want three", len(a.items()))
		}
		if got := rowState(a, 1); got != `"乙" start=11:00 end=12:00 dur=01h00m` {
			t.Errorf("row 1 = %s, want it left alone", got)
		}
	})

	t.Run("empty day", func(t *testing.T) {
		a := startLog(t)

		send(t, a, ch('o'), kt(tea.KeyEnter))

		if len(a.items()) != 1 {
			t.Fatalf("%d rows, want the one new row", len(a.items()))
		}
		if a.log.cursor != 0 || a.log.editing {
			t.Errorf("cursor = %d editing = %v, want the new row under a closed editor",
				a.log.cursor, a.log.editing)
		}
		if a.items()[0].Start == nil {
			t.Error("the new row has no start time")
		}
	})
}

func TestRowCommandsWorkFromEveryStop(t *testing.T) {
	cases := []struct {
		name string
		keys []stroke
		want func(t *testing.T, a *App, cmd tea.Cmd)
	}{
		{"dd deletes the row", typed("dd"), func(t *testing.T, a *App, _ tea.Cmd) {
			wantRows(t, a, `"甲" start=09:00 end=10:00 dur=01h00m`)
		}},
		{"y then p copies the row to the end", typed("yp"), func(t *testing.T, a *App, _ tea.Cmd) {
			wantRows(t, a,
				`"甲" start=09:00 end=10:00 dur=01h00m`,
				`"乙" start=11:00 end=12:00 dur=01h00m`,
				`"乙" start=11:00 end=12:00 dur=01h00m`)

			// The paste must not share clocks with what it was copied from.
			day := a.editableDay()
			day.Items[2].Start = mustClock("00:00")
			if got := timeText(a.items()[1].Start); got != "11:00" {
				t.Errorf("editing the copy moved the original to %s", got)
			}
		}},
		{"p with nothing copied says so", typed("p"), func(t *testing.T, a *App, _ tea.Cmd) {
			if len(a.items()) != 2 {
				t.Fatalf("%d rows, want no paste", len(a.items()))
			}
			if a.status != "剪贴板为空" {
				t.Errorf("status = %q, want the empty-clipboard note", a.status)
			}
		}},
		{"c opens the date page", typed("c"), func(t *testing.T, a *App, _ tea.Cmd) {
			if a.page != pageDates {
				t.Errorf("page = %d, want the date page", a.page)
			}
		}},
		{"? opens the help page", typed("?"), func(t *testing.T, a *App, _ tea.Cmd) {
			if a.page != pageHelp {
				t.Errorf("page = %d, want the help page", a.page)
			}
		}},
		{"q quits", typed("q"), func(t *testing.T, a *App, cmd tea.Cmd) {
			if !isQuit(t, cmd) {
				t.Error("q scheduled no quit")
			}
		}},
		{"gg goes to the top", typed("gg"), func(t *testing.T, a *App, _ tea.Cmd) {
			if a.log.cursor != 0 {
				t.Errorf("cursor = %d, want the first row", a.log.cursor)
			}
		}},
		{"G goes to the bottom", typed("G"), func(t *testing.T, a *App, _ tea.Cmd) {
			if a.log.cursor != len(a.items())-1 {
				t.Errorf("cursor = %d, want the last row", a.log.cursor)
			}
		}},
	}

	for _, c := range cases {
		for stop := range stopCount {
			t.Run(c.name+" on "+stopNames[stop], func(t *testing.T) {
				a := startLog(t, rowOf("甲", "09:00", "10:00"), rowOf("乙", "11:00", "12:00"))
				a.log.cursor = 1
				park(t, a, stop)

				cmd := send(t, a, c.keys...)

				c.want(t, a, cmd)
			})
		}
	}
}

func TestRetiredAndUnboundKeysAreInert(t *testing.T) {
	for stop := range stopCount {
		for _, r := range []rune{'.', 'e', 'v', 'u', 'n'} {
			t.Run(fmt.Sprintf("%s on %s", string(r), stopNames[stop]), func(t *testing.T) {
				a := startLog(t, rowOf("甲", "09:00", "10:00"), rowOf("乙", "11:00", "12:00"))
				a.log.cursor = 1
				park(t, a, stop)

				cmd := send(t, a, ch(r))

				if isQuit(t, cmd) {
					t.Fatal("the key quit the program")
				}
				if a.page != pageLog {
					t.Errorf("page = %d, want the log page", a.page)
				}
				if a.log.field != stop {
					t.Errorf("cursor on %s, want it to stay on %s", stopNames[a.log.field], stopNames[stop])
				}
				if a.log.cursor != 1 || a.log.editing {
					t.Errorf("cursor = %d editing = %v, want the row untouched", a.log.cursor, a.log.editing)
				}
				wantRows(t, a,
					`"甲" start=09:00 end=10:00 dur=01h00m`,
					`"乙" start=11:00 end=12:00 dur=01h00m`)
			})
		}
	}
}

func TestEveryMutationLandsOnDisk(t *testing.T) {
	cases := []struct {
		name string
		stop int
		keys []stroke
	}{
		{"stepping an hour", fStartHour, []stroke{kt(tea.KeyUp)}},
		{"a single typed tens digit", fStartHour, typed("1")},
		{"typing a minute", fStartMinute, typed("30")},
		{"stepping the span", fDurMinute, presses(tea.KeyUp, 2)},
		{"typing a span", fDurHour, typed("11")},
		{"reordering an item", fIndex, typed("J")},
		{"filling a reading from the clock", fEndHour, typed("s")},
		{"editing the content", fContent, append(typed("a改"), kt(tea.KeyEnter))},
		{"inserting a row", fContent, append(typed("o新"), kt(tea.KeyEnter))},
		{"pasting a row", fContent, typed("yp")},
		{"deleting a row", fContent, typed("dd")},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			a := startLog(t, rowOf("甲", "09:00", "10:00"), rowOf("乙", "11:00", "12:00"))
			park(t, a, c.stop)

			send(t, a, c.keys...)

			wantFile(t, a)
			if strings.Contains(a.status, "保存失败") {
				t.Errorf("status = %q, want a clean save", a.status)
			}
		})
	}
}

func TestSaveFailureIsReportedNotSwallowed(t *testing.T) {
	blocker := filepath.Join(t.TempDir(), "blocker")
	if err := os.WriteFile(blocker, []byte("这是一个文件，不是目录"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	a := startLog(t, rowOf("甲", "09:00", "10:00"))
	a.store.Path = filepath.Join(blocker, "nope", "orgmaid.org")
	park(t, a, fStartHour)

	send(t, a, kt(tea.KeyUp))

	if !strings.HasPrefix(a.status, "保存失败") {
		t.Errorf("status = %q, want a save failure", a.status)
	}
	if got := timeText(a.items()[0].Start); got != "10:00" {
		t.Errorf("start = %s, want the edit kept in memory all the same", got)
	}
}

// TestTheHintBarDescribesTheStopItSitsOn pins the whole hint of every stop: it
// is the only place a reader learns which way ↑ and ↓ move this particular
// number, so the wording is part of the behaviour.
func TestTheHintBarDescribesTheStopItSitsOn(t *testing.T) {
	cases := []struct {
		stop int
		want string
	}{
		{fIndex, "\uf0cb 序号 j/k 换项 J/K 挪本项 Tab 换列 ? 帮助 Esc 回内容"},
		{fTodo, "\uf0ae 待办 ↑/↓ 切换状态 Tab 换列 Esc 回内容"},
		{fScheduled, "\uf073 计划 ↑/↓ 打开日期选择器 s 今天 x 清空 Tab 换列 Esc 回内容"},
		{fDeadline, "\uf073 截止 ↑/↓ 打开日期选择器 s 今天 x 清空 Tab 换列 Esc 回内容"},
		{fStartHour, "开始 小时 | ↑ 加 ↓ 减 每次 1 循环 | 数字 s 现在 x 清空 | Tab 换列 Esc 回内容"},
		{fStartMinute, "开始 分钟 | ↑ 加 ↓ 减 每次 5 循环 | 数字 s 现在 x 清空 | Tab 换列 Esc 回内容"},
		{fEndHour, "结束 小时 | ↑ 加 ↓ 减 每次 1 循环 | 数字 s 现在 x 清空 | Tab 换列 Esc 回内容"},
		{fEndMinute, "结束 分钟 | ↑ 加 ↓ 减 每次 5 循环 | 数字 s 现在 x 清空 | Tab 换列 Esc 回内容"},
		{fDurHour, "耗时 小时 | ↑ 加 ↓ 减 每次 1 循环 | 数字 写回结束时间 | Tab 换列 Esc 回内容"},
		{fDurMinute, "耗时 分钟 | ↑ 加 ↓ 减 每次 5 循环 | 数字 写回结束时间 | Tab 换列 Esc 回内容"},
		{fContent, "\uf044 内容 j/k 换项 J/K 挪 i 编辑 o 新建 t 待办 T 全局 , 标签 c 日期 q 退出"},
	}

	for _, c := range cases {
		t.Run(stopNames[c.stop], func(t *testing.T) {
			a := startLog(t, rowOf("甲", "09:00", "10:00"))
			park(t, a, c.stop)

			if got := a.stopHints(); got != c.want {
				t.Errorf("hint for %s =\n%q\nwant\n%q", stopNames[c.stop], got, c.want)
			}
			hints := ansi.Strip(a.renderStatus())
			if !strings.HasPrefix(hints, c.want) {
				t.Errorf("hint bar %q does not carry the hint %q", hints, c.want)
			}
			if !strings.Contains(hints, stopNames[c.stop]) {
				t.Errorf("hint bar %q does not name the stop it describes", hints)
			}
		})
	}

	t.Run("editor", func(t *testing.T) {
		a := startLog(t, rowOf("甲", "09:00", "10:00"))
		park(t, a, fStartHour)

		send(t, a, ch('a'))

		if got := a.stopHints(); got != "\uf044 编辑 | Enter 或 Esc 完成" {
			t.Errorf("hint while editing = %q", got)
		}
		if hints := ansi.Strip(a.renderStatus()); !strings.Contains(hints, "编辑") {
			t.Errorf("hint bar %q does not say the editor is open", hints)
		}
	})
}

// TestEveryStopHintFitsAnEightyColumnTerminal is why the hints were rewritten:
// the bar is the only key reference on screen, and one cut at the edge of the
// terminal loses the key the reader was reaching for.
func TestEveryStopHintFitsAnEightyColumnTerminal(t *testing.T) {
	const terminal = 80

	check := func(t *testing.T, a *App, what string) {
		t.Helper()

		hint := a.stopHints()
		if w := lipgloss.Width(hint); w > terminal {
			t.Errorf("the %s hint is %d cells wide, want it to fit %d: %q", what, w, terminal, hint)
		}
	}

	for stop := 0; stop < stopCount; stop++ {
		t.Run(stopNames[stop], func(t *testing.T) {
			a := startLog(t, rowOf("甲", "09:00", "10:00"))
			park(t, a, stop)

			check(t, a, stopNames[stop])
		})
	}

	t.Run("editor", func(t *testing.T) {
		a := startLog(t, rowOf("甲", "09:00", "10:00"))
		send(t, a, ch('a'))

		check(t, a, "editor")
	})
}

func TestTheSelectedRowCarriesTheOnlyGround(t *testing.T) {
	useTrueColor(t)

	a := startLog(t, rowOf("甲", "09:00", "10:00"), rowOf("乙", "11:00", "12:00"))

	rows := frameRows(t, a)
	// When focus is on content (default), the content area is highlighted
	// Check that the content text is present with the selected color
	if !strings.Contains(rows[0], fgSeq(t, pal.Selected)) {
		t.Error("the row the cursor is on does not have the selected color")
	}
	if !strings.Contains(rows[0], "甲") {
		t.Error("the row the cursor is on does not contain the content")
	}
	if !strings.Contains(rows[1], fgSeq(t, pal.Text)) {
		t.Error("a row the cursor is not on is not written in the plain text accent")
	}
	if strings.Contains(rows[1], bgSeq(t, pal.Canvas)) {
		t.Error("a row the cursor is not on still sits on a canvas of its own")
	}
	if strings.Contains(rows[1], bgSeq(t, pal.Row)) {
		t.Error("a row the cursor is not on borrowed the selected row's ground")
	}

	// The block follows the stop, not the content column, and the content keeps
	// the colour its row gave it wherever the cursor walks to.
	park(t, a, fStartHour)
	rows = frameRows(t, a)
	if !strings.Contains(rows[0], painted(t, "甲", pal.Selected, pal.Row)) {
		t.Error("the content of the selected row fell back once the cursor left it")
	}

	send(t, a, ch('a'))
	rows = frameRows(t, a)
	if !strings.Contains(rows[0], fgSeq(t, pal.Editing)) {
		t.Error("the open editor does not take the Editing colour")
	}
}

func TestFieldBandPicksOutTheFocusedHalf(t *testing.T) {
	useTrueColor(t)

	field := bgSeq(t, pal.Field)
	cases := []struct {
		name   string
		stop   int
		focus  string // the half the cursor is on
		accent string // the colour the rest of the column keeps
		spare  string // the other half of the same column
	}{
		{"start hour", fStartHour, "09", pal.Start, "00"},
		{"start minute", fStartMinute, "00", pal.Start, "09"},
		{"end hour", fEndHour, "10", pal.End, "00"},
		{"end minute", fEndMinute, "00", pal.End, "10"},
		{"duration hour", fDurHour, "01", pal.Duration, "00"},
		{"duration minute", fDurMinute, "00", pal.Duration, "01"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			a := startLog(t, rowOf("甲", "09:00", "10:00"), rowOf("乙", "11:00", "12:00"))
			park(t, a, c.stop)

			rows := frameRows(t, a)

			if !strings.Contains(rows[0], painted(t, c.focus, pal.Ink, pal.Field)) {
				t.Errorf("the focused half %q is not lifted onto the cursor's block", c.focus)
			}
			if got := strings.Count(rows[0], field); got != 1 {
				t.Errorf("the cursor row paints the block %d times, want once", got)
			}
			if !strings.Contains(rows[0], painted(t, c.spare, c.accent, pal.Row)) {
				t.Errorf("the neighbour %q lost the %s accent", c.spare, c.accent)
			}
			for i, row := range rows[1:] {
				if got := strings.Count(row, field); got != 0 {
					t.Errorf("row %d paints the cursor's block %d times, want none off the cursor", i+1, got)
				}
			}
		})
	}
}

func TestEveryStopKeepsTheFrameInsideTheTerminal(t *testing.T) {
	a := startLog(t, rowOf("一段很长很长的中文内容，用来检查行尾的裁切", "09:00", "10:00"))

	for stop := range stopCount {
		park(t, a, stop)

		for n, line := range strings.Split(a.View(), "\n") {
			if w := ansi.StringWidth(line); w > 80 {
				t.Errorf("cursor on %s draws a %d cell line %d: %q", stopNames[stop], w, n+1, line)
			}
		}
	}
}

func TestAnEmptyDayToleratesEveryKey(t *testing.T) {
	strokes := []stroke{
		kt(tea.KeyUp), kt(tea.KeyDown), kt(tea.KeyEnter), kt(tea.KeyEsc),
		kt(tea.KeyTab), kt(tea.KeyShiftTab),
		ch('k'), ch('j'), ch('K'), ch('J'), ch('h'), ch('l'),
		ch('0'), ch('5'), ch('s'), ch('.'), ch('G'),
		ch('i'), ch('a'), ch('y'), ch('p'), ch('g'), ch('d'),
	}

	for stop := range stopCount {
		t.Run(stopNames[stop], func(t *testing.T) {
			a := startLog(t)
			park(t, a, stop)

			for _, k := range strokes {
				send(t, a, k)

				if got := len(a.items()); got != 0 {
					t.Fatalf("%v on an empty day invented %d rows", k, got)
				}
				if a.log.editing {
					t.Fatalf("%v opened the editor on a day with no row to edit", k)
				}
			}
		})
	}
}

func TestAnEmptyDaySaysHowToStart(t *testing.T) {
	a := startLog(t)

	if plain := ansi.Strip(a.viewLog()); !strings.Contains(plain, "今日暂无记录") {
		t.Errorf("frame does not offer the way to start a day:\n%s", plain)
	}
}

func TestAPendingKeyIsEchoedInTheHintBar(t *testing.T) {
	cases := []struct {
		name string
		keys []stroke
		want string
	}{
		{"a lone d waits for its partner", typed("d"), "[d]"},
		{"a lone g waits for its partner", typed("g"), "[g]"},
		{"a digit keeps counting", typed("1"), "[1]"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			a := startLog(t, rowOf("甲", "09:00", "10:00"), rowOf("乙", "11:00", "12:00"))

			send(t, a, c.keys...)

			hints := ansi.Strip(a.renderStatus())
			if !strings.HasPrefix(hints, c.want) {
				t.Errorf("hint bar %q does not start with the pending key %q", hints, c.want)
			}
			if !strings.Contains(hints, stopNames[fContent]) {
				t.Errorf("hint bar %q stopped naming the stop the cursor is on", hints)
			}
		})
	}
}

func TestTheTickExpiresAPendingPrefix(t *testing.T) {
	a := startLog(t, rowOf("甲", "09:00", "10:00"), rowOf("乙", "11:00", "12:00"))
	a.log.cursor = 1

	started := time.Now()
	if cmd := send(t, a, ch('d')); cmd == nil {
		t.Fatal("a lone d scheduled no tick")
	}
	a.onTick(started.Add(keys.PrefixTimeout + time.Second))

	send(t, a, ch('d'))

	if len(a.items()) != 2 {
		t.Errorf("%d rows, want a stale d forgotten rather than completed", len(a.items()))
	}
}

func TestACrossedSpanKeepsItsOwnAccent(t *testing.T) {
	useTrueColor(t)

	a := startLog(t, rowOf("夜班", "23:00", "01:00"), rowOf("白天", "09:00", "10:00"))

	rows := frameRows(t, a)
	if !strings.Contains(rows[0], painted(t, "02", pal.Crossed, pal.Row)) {
		t.Error("a span running past midnight is not written in the Crossed accent")
	}
	if !strings.Contains(rows[1], fgSeq(t, pal.Duration)) {
		t.Error("an ordinary span lost the duration accent")
	}

	park(t, a, fDurHour)
	rows = frameRows(t, a)
	if !strings.Contains(rows[0], painted(t, "02", pal.Ink, pal.Field)) {
		t.Error("the focused half of a crossed span is not lifted onto the cursor's block")
	}
	if !strings.Contains(rows[0], painted(t, "h", pal.Crossed, pal.Row)) {
		t.Error("the rest of a crossed span lost the Crossed accent")
	}
}

// TestAPlaceholderStaysInTheBackground keeps a reading that is not there from
// shouting as loud as one that is: an unset clock and an uncomputable span are
// written in the secondary colour, on the selected row's ground or on the
// program ground, and no column accent is spent on them. The half the cursor
// stands on still takes the block, because an empty clock is as fillable as a
// full one.
func TestAPlaceholderStaysInTheBackground(t *testing.T) {
	useTrueColor(t)

	a := startLog(t, rowOf("还没填时间", "", ""), rowOf("也没填", "", ""))
	rows := frameRows(t, a)

	if n := strings.Count(rows[0], painted(t, "--", pal.Dim, pal.Row)); n != 6 {
		t.Errorf("row 0 shows %d placeholder halves in the secondary colour, want six: start, end and the span, two each", n)
	}
	if n := strings.Count(rows[1], paintedOnNothing(t, "--", pal.Dim)); n != 6 {
		t.Errorf("row 1 shows %d placeholder halves in the secondary colour, want six: start, end and the span, two each", n)
	}
	for _, accent := range []string{pal.Start, pal.End, pal.Duration, pal.Crossed} {
		if strings.Contains(rows[1], fgSeq(t, accent)) {
			t.Errorf("a column with nothing to show still spends its accent %s", accent)
		}
	}

	park(t, a, fStartHour)
	rows = frameRows(t, a)
	if !strings.Contains(rows[0], painted(t, "--", pal.Ink, pal.Field)) {
		t.Error("the focused half of an empty clock is not lifted onto the cursor's block")
	}
}

func TestTheListScrollsToKeepTheCursorInView(t *testing.T) {
	var many []store.Item
	for i := range 10 {
		many = append(many, rowOf(fmt.Sprintf("项%d", i+1), "09:00", "09:30"))
	}
	a := startLog(t, many...)
	if _, cmd := a.Update(tea.WindowSizeMsg{Width: 80, Height: 8}); cmd != nil {
		t.Fatal("resizing scheduled a command")
	}
	if got, want := a.listHeight(), 4; got != want {
		t.Fatalf("the list shows %d rows, want %d", got, want)
	}

	send(t, a, ch('G'))

	if a.log.cursor != 9 {
		t.Fatalf("cursor = %d, want the last row", a.log.cursor)
	}
	if got, want := a.log.offset, 9-4+1; got != want {
		t.Errorf("the list scrolls from row %d, want %d to bring the cursor into view", got, want)
	}
	joined := ansi.Strip(strings.Join(frameRows(t, a)[:4], "\n"))
	if !strings.Contains(joined, "项7") {
		t.Errorf("the first visible row is not the seventh item:\n%s", joined)
	}
	if strings.Contains(joined, "项6") {
		t.Errorf("row 6 is still on screen, want it scrolled out:\n%s", joined)
	}
}

func TestATerminalNarrowerThanTheColumnsStillLaysOut(t *testing.T) {
	a := startLog(t, rowOf("一段内容", "09:00", "10:00"))
	if _, cmd := a.Update(tea.WindowSizeMsg{Width: 20, Height: 10}); cmd != nil {
		t.Fatal("resizing scheduled a command")
	}

	for stop := 0; stop < stopCount; stop++ {
		park(t, a, stop)

		lines := strings.Split(a.View(), "\n")
		if got := len(lines); got != 10 {
			t.Fatalf("cursor on %s draws %d lines, want the terminal filled", stopNames[stop], got)
		}
		for n, line := range lines {
			if w := ansi.StringWidth(line); w > 20 {
				t.Errorf("cursor on %s draws a %d cell line %d in a 20 cell terminal: %q",
					stopNames[stop], w, n+1, line)
			}
		}
	}
}

func TestEnterDayResetsWhereTheCursorStands(t *testing.T) {
	a := startLog(t, rowOf("今天", "09:00", "10:00"), rowOf("今天还有", "11:00", "12:00"))
	a.store.Journal.EnsureDay("2026-09-10")
	tomorrow := a.store.Day("2026-09-10")
	tomorrow.Items = append(tomorrow.Items, rowOf("明天", "11:00", "11:30"))

	a.log.cursor = 1
	a.log.offset = 1
	park(t, a, fDurMinute)

	// Both halves of the row state are filled in while the log page is up, so
	// what follows really does test the day switch and not some other page.
	send(t, a, ch('1')) // the tens of the span: the ones is next
	if a.log.tensNext {
		t.Fatal("turn = the tens, want the accepted digit to pass it to the ones")
	}
	send(t, a, ch('d')) // a half-typed dd the day switch has to forget
	if got := a.pendingText(); got != "d" {
		t.Fatalf("pending = %q, want the log page to be holding the prefix", got)
	}

	a.page = pageHelp
	a.enterDay("2026-09-10")

	if a.date != "2026-09-10" || a.page != pageLog {
		t.Errorf("showing %s on page %d, want the log page for 2026-09-10", a.date, a.page)
	}
	if a.log.field != fContent || a.log.editing {
		t.Errorf("cursor on %s editing=%v, want the content stop with the editor closed",
			stopNames[a.log.field], a.log.editing)
	}
	if a.log.cursor != 0 || a.log.offset != 0 {
		t.Errorf("cursor = %d offset = %d, want the top of the list", a.log.cursor, a.log.offset)
	}
	if !a.log.tensNext {
		t.Error("turn = the ones, want the day switch to restart digit entry at the tens")
	}
	if got := a.pendingText(); got != "" {
		t.Errorf("pending = %q, want the day switch to have dropped the prefix", got)
	}
	wantRows(t, a, `"明天" start=11:00 end=11:30 dur=00h30m`)

	send(t, a, ch('d'))
	if len(a.items()) != 1 {
		t.Errorf("%d rows, want a stale dd forgotten rather than completed", len(a.items()))
	}
}

// useTrueColor switches lipgloss on for one test and puts it back afterwards.
func useTrueColor(t *testing.T) {
	t.Helper()

	lipgloss.SetColorProfile(termenv.TrueColor)
	t.Cleanup(func() { lipgloss.SetColorProfile(termenv.Ascii) })
}

// bgSeq is the escape fragment lipgloss writes for a background colour. It is
// read back out of a reference render rather than assembled by hand, because
// lipgloss carries a hex colour through a float and a hand-built triple can be
// out by one.
func bgSeq(t *testing.T, hexColor string) string {
	t.Helper()

	sample := lipgloss.NewStyle().Background(bg(hexColor)).Render("X")
	i := strings.Index(sample, "X")
	if i <= 0 {
		t.Fatalf("nothing painted for %q, want a truecolour background:\n%q", hexColor, sample)
	}
	return strings.TrimSuffix(strings.TrimPrefix(sample[:i], "\x1b["), "m")
}

// fgSeq is the same fragment for a foreground colour, which is what a column
// carries now that the rows have no bands to put it on.
func fgSeq(t *testing.T, hexColor string) string {
	t.Helper()

	sample := lipgloss.NewStyle().Foreground(fg(hexColor)).Render("X")
	i := strings.Index(sample, "X")
	if i <= 0 {
		t.Fatalf("nothing painted for %q, want a truecolour foreground:\n%q", hexColor, sample)
	}
	return strings.TrimSuffix(strings.TrimPrefix(sample[:i], "\x1b["), "m")
}

// paintedOnNothing quotes a stretch of text in one colour over the terminal's
// own background, which is what an unselected row carries now that the program
// lays no canvas of its own.
func paintedOnNothing(t *testing.T, text, ink string) string {
	t.Helper()
	return "\x1b[" + fgSeq(t, ink) + "m" + text
}

// painted quotes a stretch of text together with the colour it is written in and
// the ground it sits on, with the reset at the end dropped so the frame's own
// padding can follow. Reading it out of a reference render keeps the order
// lipgloss writes the two colours in from being something a test has to know.
func painted(t *testing.T, text, ink, ground string) string {
	t.Helper()

	rendered := lipgloss.NewStyle().Foreground(fg(ink)).Background(bg(ground)).Render(text)
	return strings.TrimSuffix(rendered, "\x1b[0m")
}
