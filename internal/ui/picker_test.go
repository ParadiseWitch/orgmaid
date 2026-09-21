package ui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestPickerDateOnly(t *testing.T) {
	p := NewPicker("2026-09-15", PickDateOnly, nil)
	p.Init(80, 24)

	if got := p.SelectedDate(); got != "2026-09-15" {
		t.Errorf("initial date = %s, want 2026-09-15", got)
	}

	// Move right one day
	handled, done := p.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'l'}})
	if !handled || done {
		t.Errorf("l key: handled=%v done=%v, want handled=true done=false", handled, done)
	}
	if got := p.SelectedDate(); got != "2026-09-16" {
		t.Errorf("after l: date = %s, want 2026-09-16", got)
	}

	// Move down one week
	handled, done = p.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	if !handled || done {
		t.Errorf("j key: handled=%v done=%v", handled, done)
	}
	if got := p.SelectedDate(); got != "2026-09-23" {
		t.Errorf("after j: date = %s, want 2026-09-23", got)
	}

	// Enter confirms
	handled, done = p.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if !handled || !done {
		t.Errorf("Enter: handled=%v done=%v, want both true", handled, done)
	}
}

func TestPickerDateTime(t *testing.T) {
	p := NewPicker("2026-09-15", PickDateTime, nil)
	p.Init(80, 24)

	// Tab to hour
	handled, _ := p.Update(tea.KeyMsg{Type: tea.KeyTab})
	if !handled || p.Focus != FocusHour {
		t.Errorf("Tab: handled=%v focus=%d, want focus=FocusHour", handled, p.Focus)
	}

	// Type 14 for hour
	p.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'1'}})
	p.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'4'}})
	if p.hour != 14 {
		t.Errorf("after typing 14: hour = %d, want 14", p.hour)
	}

	// Tab to minute
	p.Update(tea.KeyMsg{Type: tea.KeyTab})
	if p.Focus != FocusMinute {
		t.Errorf("Tab: focus = %d, want FocusMinute", p.Focus)
	}

	// Type 30 for minute
	p.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'3'}})
	p.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'0'}})
	if p.minute != 30 {
		t.Errorf("after typing 30: minute = %d, want 30", p.minute)
	}

	// Check selected time
	ts := p.SelectedTime()
	if ts.Hour != 14 || ts.Minute != 30 {
		t.Errorf("SelectedTime = %02d:%02d, want 14:30", ts.Hour, ts.Minute)
	}
}

func TestPickerTimeArrows(t *testing.T) {
	p := NewPicker("2026-09-15", PickDateTime, nil)
	p.Init(80, 24)
	p.SetTime(10, 15)

	// Tab to hour
	p.Update(tea.KeyMsg{Type: tea.KeyTab})

	// Up adds 1 hour
	p.Update(tea.KeyMsg{Type: tea.KeyUp})
	if p.hour != 11 {
		t.Errorf("Up on hour: %d, want 11", p.hour)
	}

	// Tab to minute
	p.Update(tea.KeyMsg{Type: tea.KeyTab})

	// Up adds 5 minutes
	p.Update(tea.KeyMsg{Type: tea.KeyUp})
	if p.minute != 20 {
		t.Errorf("Up on minute: %d, want 20", p.minute)
	}

	// Down takes 5 minutes
	p.Update(tea.KeyMsg{Type: tea.KeyDown})
	if p.minute != 15 {
		t.Errorf("Down on minute: %d, want 15", p.minute)
	}
}

func TestPickerSetNow(t *testing.T) {
	p := NewPicker("2026-09-15", PickDateTime, nil)
	p.Init(80, 24)

	// Tab to hour
	p.Update(tea.KeyMsg{Type: tea.KeyTab})

	// s sets to now
	p.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'s'}})

	// Hour and minute should be close to current time
	if p.hour < 0 || p.hour > 23 {
		t.Errorf("after s: hour = %d, out of range", p.hour)
	}
	if p.minute < 0 || p.minute > 59 {
		t.Errorf("after s: minute = %d, out of range", p.minute)
	}
}

func TestPickerMonthNavigation(t *testing.T) {
	p := NewPicker("2026-09-15", PickDateOnly, nil)
	p.Init(80, 24)

	// H moves back one month
	p.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'H'}})
	if p.calMonth != 8 {
		t.Errorf("after H: month = %d, want 8", p.calMonth)
	}

	// L moves forward one month
	p.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'L'}})
	if p.calMonth != 9 {
		t.Errorf("after L: month = %d, want 9", p.calMonth)
	}
}

func TestPickerJumpToToday(t *testing.T) {
	p := NewPicker("2020-01-01", PickDateOnly, nil)
	p.Init(80, 24)

	// s jumps to today
	p.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'s'}})

	if p.SelectedDate() == "2020-01-01" {
		t.Error("after s: date unchanged, want today")
	}
}

func TestPickerEscReturnsToCalendar(t *testing.T) {
	p := NewPicker("2026-09-15", PickDateTime, nil)
	p.Init(80, 24)

	// Tab to hour
	p.Update(tea.KeyMsg{Type: tea.KeyTab})
	if p.Focus != FocusHour {
		t.Fatalf("Tab: focus = %d, want FocusHour", p.Focus)
	}

	// Esc returns to calendar
	p.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if p.Focus != FocusCalendar {
		t.Errorf("Esc: focus = %d, want FocusCalendar", p.Focus)
	}
}
