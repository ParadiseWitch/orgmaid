package ui

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"orgmaid/internal/store"
)

// PickerMode controls what the picker lets the user choose.
type PickerMode int

const (
	// PickDateOnly shows only the calendar, no time input.
	PickDateOnly PickerMode = iota
	// PickDateTime shows the calendar plus an hour:minute input below it.
	PickDateTime
)

// PickerFocus names which part of the picker has the cursor.
type PickerFocus int

const (
	FocusCalendar PickerFocus = iota
	FocusHour
	FocusMinute
)

// Picker is a reusable date (and optional time) selector. It draws a calendar
// for the date and, in DateTime mode, an HH:MM input underneath. The caller
// reads SelectedDate and SelectedTime after the picker returns.
type Picker struct {
	// Mode is set before Init and decides whether the time row is drawn.
	Mode PickerMode

	// Focus is which part of the picker the cursor sits on.
	Focus PickerFocus

	// calYear/calMonth is the month the calendar shows.
	calYear  int
	calMonth int
	// calCursor is the date the calendar cursor stands on.
	calCursor string

	// hour/minute are the time the user is picking.
	hour   int
	minute int

	// tensNext is whose turn the next typed digit is on a time stop.
	tensNext bool

	// counts marks dates that have entries, drawn as a dot under the day.
	counts map[string]int

	// width/height are the terminal size, set by Init.
	width  int
	height int
}

// NewPicker builds a picker starting on the given date (or today if empty).
// counts may be nil when no dates are marked.
func NewPicker(date string, mode PickerMode, counts map[string]int) *Picker {
	year, month, _ := parseDateParts(date)
	if date == "" {
		today := store.Today()
		year, month, _ = parseDateParts(today)
		date = today
	}
	return &Picker{
		Mode:      mode,
		Focus:     FocusCalendar,
		calYear:   year,
		calMonth:  month,
		calCursor: date,
		counts:    counts,
	}
}

// Init sizes the picker to the terminal.
func (p *Picker) Init(width, height int) {
	p.width = width
	p.height = height
}

// SelectedDate is the date the calendar cursor stands on.
func (p *Picker) SelectedDate() string { return p.calCursor }

// SelectedTime fills a Timestamp with the date and the time the picker holds.
func (p *Picker) SelectedTime() store.Timestamp {
	ts, _ := store.TimestampFromDateTime(p.calCursor, p.hour, p.minute)
	return ts
}

// SetTime writes an initial time into the picker, so a caller that edits an
// existing timestamp can hand the current reading in.
func (p *Picker) SetTime(hour, minute int) {
	p.hour = hour
	p.minute = minute
}

// Update handles one key and reports whether the picker took it. A false return
// means the caller should keep the key for itself (Esc on the calendar, for
// instance, is the caller's business, not the picker's).
func (p *Picker) Update(msg tea.KeyMsg) (handled bool, done bool) {
	switch p.Focus {
	case FocusCalendar:
		return p.updateCalendar(msg)
	case FocusHour, FocusMinute:
		return p.updateTime(msg)
	}
	return false, false
}

// updateCalendar handles keys on the calendar. done is true when the user
// pressed Enter to confirm; handled is true for any key the calendar ate.
func (p *Picker) updateCalendar(msg tea.KeyMsg) (bool, bool) {
	if r, ok := singleRune(msg); ok {
		switch r {
		case 'h':
			p.moveCalendar(-1)
			return true, false
		case 'l':
			p.moveCalendar(1)
			return true, false
		case 'j':
			p.moveCalendar(7)
			return true, false
		case 'k':
			p.moveCalendar(-7)
			return true, false
		case 'H', 'K':
			p.moveCalendarMonth(-1)
			return true, false
		case 'L', 'J':
			p.moveCalendarMonth(1)
			return true, false
		case 's':
			today := store.Today()
			p.calCursor = today
			p.calYear, p.calMonth, _ = parseDateParts(today)
			return true, false
		case 't':
			if p.Mode == PickDateTime {
				p.Focus = FocusHour
				p.tensNext = true
				return true, false
			}
		}
	}

	switch msg.Type {
	case tea.KeyLeft:
		p.moveCalendar(-1)
		return true, false
	case tea.KeyRight:
		p.moveCalendar(1)
		return true, false
	case tea.KeyUp:
		p.moveCalendar(-7)
		return true, false
	case tea.KeyDown:
		p.moveCalendar(7)
		return true, false
	case tea.KeyTab:
		if p.Mode == PickDateTime {
			p.Focus = FocusHour
			p.tensNext = true
			return true, false
		}
		return false, false
	case tea.KeyEnter:
		return true, true
	}
	return false, false
}

// updateTime handles keys on the hour or minute stop.
func (p *Picker) updateTime(msg tea.KeyMsg) (bool, bool) {
	if r, ok := singleRune(msg); ok {
		switch {
		case r >= '0' && r <= '9':
			p.typeTimeDigit(int(r - '0'))
			return true, false
		case r == 's':
			now := store.NowTimestamp()
			p.hour = now.Hour
			p.minute = now.Minute
			p.tensNext = true
			return true, false
		case r == 'x':
			if p.Focus == FocusHour {
				p.hour = 0
			} else {
				p.minute = 0
			}
			p.tensNext = true
			return true, false
		}
	}

	switch msg.Type {
	case tea.KeyUp:
		if p.Focus == FocusHour {
			p.hour = wrap(p.hour+1, 24)
		} else {
			p.minute = wrap(p.minute+5, 60)
		}
		return true, false
	case tea.KeyDown:
		if p.Focus == FocusHour {
			p.hour = wrap(p.hour-1, 24)
		} else {
			p.minute = wrap(p.minute-5, 60)
		}
		return true, false
	case tea.KeyTab:
		if p.Focus == FocusHour {
			p.Focus = FocusMinute
			p.tensNext = true
		} else {
			p.Focus = FocusCalendar
		}
		return true, false
	case tea.KeyShiftTab:
		if p.Focus == FocusMinute {
			p.Focus = FocusHour
			p.tensNext = true
		} else {
			p.Focus = FocusCalendar
		}
		return true, false
	case tea.KeyEsc:
		p.Focus = FocusCalendar
		return true, false
	case tea.KeyEnter:
		return true, true
	}
	return false, false
}

// typeTimeDigit writes one digit into the focused time stop.
func (p *Picker) typeTimeDigit(d int) {
	limit := 59
	if p.Focus == FocusHour {
		limit = 23
	}
	value := p.hour
	if p.Focus == FocusMinute {
		value = p.minute
	}

	next := value/10*10 + d
	if p.tensNext {
		next = d*10 + value%10
	}
	if next > limit {
		return
	}

	if p.Focus == FocusHour {
		p.hour = next
	} else {
		p.minute = next
	}
	p.tensNext = !p.tensNext
}

// moveCalendar shifts the calendar cursor by delta days.
func (p *Picker) moveCalendar(deltaDays int) {
	t, _ := time.Parse("2006-01-02", p.calCursor)
	t = t.AddDate(0, 0, deltaDays)
	p.calCursor = t.Format("2006-01-02")
	p.calYear = t.Year()
	p.calMonth = int(t.Month())
}

// moveCalendarMonth shifts the calendar by months.
func (p *Picker) moveCalendarMonth(deltaMonths int) {
	t, _ := time.Parse("2006-01-02", p.calCursor)
	t = t.AddDate(0, deltaMonths, 0)
	p.calCursor = t.Format("2006-01-02")
	p.calYear = t.Year()
	p.calMonth = int(t.Month())
}

// View draws the picker.
func (p *Picker) View() string {
	var rows []string
	rows = append(rows, p.viewCalendar())
	if p.Mode == PickDateTime {
		rows = append(rows, divider(p.width))
		rows = append(rows, p.viewTime())
	}
	return lipgloss.JoinVertical(lipgloss.Left, rows...)
}

// viewCalendar draws the month grid.
func (p *Picker) viewCalendar() string {
	year, month := p.calYear, p.calMonth

	calWidth := 21
	calLeft := (p.width - calWidth) / 2
	if calLeft < 0 {
		calLeft = 0
	}
	leftPad := strings.Repeat(" ", calLeft)

	header := titleStyle.Render(fmt.Sprintf("\uf073 %d年 %s", year, monthName(month)))
	headerWidth := lipgloss.Width(header)
	headerLeft := (p.width - headerWidth) / 2
	if headerLeft < 0 {
		headerLeft = 0
	}
	titleRow := strings.Repeat(" ", headerLeft) + header

	weekdays := "日 一 二 三 四 五 六"
	weekdayRow := leftPad + dimStyle.Render(weekdays)

	firstDay := firstDayOfMonth(year, month)
	totalDays := daysInMonth(year, month)

	var grid []string
	grid = append(grid, titleRow)
	grid = append(grid, divider(p.width))
	grid = append(grid, weekdayRow)

	day := 1
	for week := 0; week < 6 && day <= totalDays; week++ {
		var weekLine strings.Builder
		weekLine.WriteString(leftPad)

		for dow := 0; dow < 7; dow++ {
			if (week == 0 && dow < firstDay) || day > totalDays {
				weekLine.WriteString("   ")
			} else {
				dateStr := formatDate(year, month, day)
				isToday := dateStr == store.Today()
				isSelected := dateStr == p.calCursor
				hasEntry := p.counts != nil && p.counts[dateStr] > 0

				var style lipgloss.Style
				var text string

				if isSelected {
					style = lipgloss.NewStyle().
						Background(bg(pal.Field)).
						Foreground(fg(pal.Ink)).
						Bold(true)
					text = fmt.Sprintf("%2d", day)
				} else if isToday {
					style = lipgloss.NewStyle().
						Foreground(fg(pal.Warn)).
						Bold(true).
						Underline(true)
					text = fmt.Sprintf("%2d", day)
				} else if hasEntry {
					style = lipgloss.NewStyle().
						Foreground(fg(pal.Start))
					text = fmt.Sprintf("%2d\u0323", day)
				} else {
					style = lipgloss.NewStyle().
						Foreground(fg(pal.Text))
					text = fmt.Sprintf("%2d", day)
				}

				weekLine.WriteString(style.Render(text))
				weekLine.WriteString(" ")
				day++
			}
		}
		grid = append(grid, weekLine.String())
	}

	// Info line about the selected date
	if p.calCursor != "" {
		count := 0
		if p.counts != nil {
			count = p.counts[p.calCursor]
		}
		weekday := store.Weekday(p.calCursor)
		info := fmt.Sprintf("%s %s | %d 项", p.calCursor, weekday, count)
		if p.calCursor == store.Today() {
			info += " (今天)"
		}
		grid = append(grid, divider(p.width))
		grid = append(grid, cell(info, p.width, lipgloss.Center, fg(pal.Dim), transparent))
	}

	return lipgloss.JoinVertical(lipgloss.Left, grid...)
}

// viewTime draws the HH:MM input row below the calendar.
func (p *Picker) viewTime() string {
	hourText := fmt.Sprintf("%02d", p.hour)
	minuteText := fmt.Sprintf("%02d", p.minute)

	hourActive := p.Focus == FocusHour
	minuteActive := p.Focus == FocusMinute

	ground := transparent
	hourCol := fg(pal.Duration)
	minuteCol := fg(pal.Duration)
	if hourActive {
		hourCol = fg(pal.Ink)
	}
	if minuteActive {
		minuteCol = fg(pal.Ink)
	}

	hourBg := ground
	minuteBg := ground
	if hourActive {
		hourBg = bg(pal.Field)
	}
	if minuteActive {
		minuteBg = bg(pal.Field)
	}

	label := dimStyle.Render("时间 ")
	hour := lipgloss.NewStyle().
		Foreground(hourCol).
		Background(hourBg).
		Render(hourText)
	sep := dimStyle.Render(":")
	minute := lipgloss.NewStyle().
		Foreground(minuteCol).
		Background(minuteBg).
		Render(minuteText)

	timeRow := label + hour + sep + minute

	// Hints on the right
	var hint string
	switch p.Focus {
	case FocusHour:
		hint = "小时 | ↑↓ 加减 数字输入 s 现在 Tab 分钟 Esc 回日历"
	case FocusMinute:
		hint = "分钟 | ↑↓ 加减 数字输入 s 现在 Tab 日历 Esc 回日历"
	default:
		hint = "Tab 输入时间 Enter 确认"
	}

	row := spread(p.width, timeRow, dimStyle.Render(hint))
	return row
}

// singleRune extracts a single rune from a key message, or reports false.
func singleRune(msg tea.KeyMsg) (rune, bool) {
	if msg.Type == tea.KeyRunes && len(msg.Runes) == 1 {
		return msg.Runes[0], true
	}
	return 0, false
}
