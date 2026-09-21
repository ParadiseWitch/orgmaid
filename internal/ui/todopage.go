package ui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"orgmaid/internal/keys"
	"orgmaid/internal/store"
)

type todoEntry struct {
	date  string
	index int
	item  *store.Item
}

type todoState struct {
	entries []todoEntry
	cursor  int
	offset  int
}

func (a *App) collectTodos() []todoEntry {
	var entries []todoEntry
	j := a.journal()
	for _, day := range j.Days {
		for i := range day.Items {
			item := &day.Items[i]
			// Include items with TODO status, or with SCHEDULED/DEADLINE dates
			if item.Todo == "TODO" || item.Scheduled != nil || item.Deadline != nil {
				entries = append(entries, todoEntry{
					date:  day.Date,
					index: i,
					item:  item,
				})
			}
		}
	}
	return entries
}

func (a *App) openTodos() {
	a.todos.entries = a.collectTodos()
	a.todos.cursor = 0
	a.todos.offset = 0
	a.page = pageTodos
}

func (a *App) updateTodos(k tea.KeyMsg) tea.Cmd {
	a.status = ""
	t := &a.todos

	if r, ok := keys.SingleRune(k); ok {
		switch r {
		case 'j':
			if len(t.entries) > 0 {
				t.cursor = clamp(t.cursor+1, 0, len(t.entries)-1)
			}
			return nil
		case 'k':
			if len(t.entries) > 0 {
				t.cursor = clamp(t.cursor-1, 0, len(t.entries)-1)
			}
			return nil
		case 't':
			if len(t.entries) > 0 && t.cursor < len(t.entries) {
				entry := t.entries[t.cursor]
				switch entry.item.Todo {
				case "TODO":
					entry.item.Todo = "DONE"
					a.status = "标记为完成"
				case "DONE":
					entry.item.Todo = ""
					a.status = "取消标记"
				}
				a.save()
				// Refresh the list
				a.todos.entries = a.collectTodos()
				if t.cursor >= len(t.entries) {
					t.cursor = len(t.entries) - 1
				}
			}
			return nil
		case 'q', '?':
			a.page = pageLog
			return nil
		}
	}

	switch k.Type {
	case tea.KeyDown:
		if len(t.entries) > 0 {
			t.cursor = clamp(t.cursor+1, 0, len(t.entries)-1)
		}
	case tea.KeyUp:
		if len(t.entries) > 0 {
			t.cursor = clamp(t.cursor-1, 0, len(t.entries)-1)
		}
	case tea.KeyEnter:
		if len(t.entries) > 0 && t.cursor < len(t.entries) {
			entry := t.entries[t.cursor]
			a.date = entry.date
			a.log.cursor = entry.index
			a.page = pageLog
		}
	case tea.KeyEsc:
		a.page = pageLog
	case tea.KeyCtrlC:
		return tea.Quit
	}
	return nil
}

func (a *App) viewTodos() string {
	t := &a.todos
	height := a.listHeight()

	rows := make([]string, 0, height)
	if len(t.entries) == 0 {
		hint := dimStyle.Render(strings.Repeat(" ", rowMargin) + "没有待办事项")
		rows = append(rows, lipgloss.NewStyle().Width(a.width).Render(fit(hint, a.width)))
	} else {
		for i := t.offset; i < len(t.entries) && len(rows) < height; i++ {
			rows = append(rows, a.renderTodoRow(i, t.entries[i]))
		}
	}
	for len(rows) < height {
		rows = append(rows, "")
	}

	return lipgloss.JoinVertical(lipgloss.Left,
		spread(a.width, titleStyle.Render("\uf0ae 待办事项"), dimStyle.Render(fmt.Sprintf("%d 项", len(t.entries)))),
		divider(a.width),
		lipgloss.JoinVertical(lipgloss.Left, rows...),
		divider(a.width),
		statusStyle.Width(a.width).Render(fit("待办 | j/k 选择 | t 完成 | Enter 跳转 | Esc 返回 | q 退出", a.width)),
	)
}

func (a *App) renderTodoRow(i int, entry todoEntry) string {
	selected := i == a.todos.cursor

	ground := transparent
	if selected {
		ground = bg(pal.Row)
	}

	marker := "  "
	if selected {
		marker = "\uf0da "
	}

	dateFG := fg(pal.Text)
	contentFG := fg(pal.Selected)
	if !selected {
		dateFG = fg(pal.Dim)
		contentFG = fg(pal.Text)
	}

	content := entry.item.Content
	if content == "" {
		content = "（无内容）"
	}
	if len(entry.item.Tags) > 0 {
		content += "  :" + strings.Join(entry.item.Tags, ":") + ":"
	}

	// TODO status in first column (with spaces inside highlight + separator spaces outside)
	todoWidth := 0
	todoStr := ""
	if entry.item.Todo != "" {
		todoStr = entry.item.Todo
		todoWidth = len(todoStr) + 4 // +2 for spaces inside highlight + +2 for separator spaces outside
	}

	// SCHEDULED/DEADLINE in last column (with spaces inside highlight + separator spaces outside)
	var dateParts []string
	if entry.item.Scheduled != nil {
		dateParts = append(dateParts, "S:"+shortDate(entry.item.Scheduled.DateString()))
	}
	if entry.item.Deadline != nil {
		dateParts = append(dateParts, "D:"+shortDate(entry.item.Deadline.DateString()))
	}
	dateStatus := strings.Join(dateParts, " ")
	dateWidth := 0
	if dateStatus != "" {
		// Each part gets spaces inside highlight + separator spaces outside
		dateWidth = len(dateStatus) + 4 // +2 for outer separator spaces + +2 for inner spaces
		if len(dateParts) > 1 {
			dateWidth += 2 // +2 for inner spaces between parts
		}
	}

	contentWidth := a.width - rowMargin - 2 - todoWidth - 10 - 1 - dateWidth
	if contentWidth < 1 {
		contentWidth = 1
	}

	// Build colored TODO string with background (including spaces) - same colors as log page
	todoRendered := ""
	if entry.item.Todo != "" {
		if entry.item.Todo == "DONE" {
			todoRendered = run(" ", fg(pal.Text), ground) +
				lipgloss.NewStyle().
					Foreground(fg(pal.Ink)).
					Background(bg("#6aaa7a")).
					Render(" "+entry.item.Todo+" ") +
				run(" ", fg(pal.Text), ground)
		} else {
			todoRendered = run(" ", fg(pal.Text), ground) +
				lipgloss.NewStyle().
					Foreground(fg(pal.Ink)).
					Background(bg("#5b8abf")).
					Render(" "+entry.item.Todo+" ") +
				run(" ", fg(pal.Text), ground)
		}
	}

	// Build colored S/D string with backgrounds (including spaces) - same colors as log page
	dateRendered := ""
	if dateStatus != "" {
		dateRendered = run(" ", fg(pal.Text), ground) + a.renderTodoDateStatus(entry.item, selected) + run(" ", fg(pal.Text), ground)
	}

	row := run(strings.Repeat(" ", rowMargin), fg(pal.Text), ground) +
		run(marker, fg(pal.Warn), ground) +
		todoRendered +
		lipgloss.JoinHorizontal(lipgloss.Top,
			cell(entry.date, 10, lipgloss.Left, dateFG, ground),
			run(" ", fg(pal.Text), ground),
			cell(content, contentWidth, lipgloss.Left, contentFG, ground),
		) +
		dateRendered

	return cut(row, a.width)
}

// shortDate converts "2026-09-21" to "09-21"
func shortDate(date string) string {
	if len(date) >= 10 {
		return date[5:]
	}
	return date
}

// renderTodoDateStatus builds the colored S/D status string with backgrounds (including spaces) - same colors as log page
func (a *App) renderTodoDateStatus(item *store.Item, selected bool) string {
	var parts []string
	if item.Scheduled != nil {
		parts = append(parts, lipgloss.NewStyle().
			Foreground(fg(pal.Ink)).
			Background(bg(pal.Start)).
			Render(" S:"+shortDate(item.Scheduled.DateString())+" "))
	}
	if item.Deadline != nil {
		parts = append(parts, lipgloss.NewStyle().
			Foreground(fg(pal.Ink)).
			Background(bg(pal.Crossed)).
			Render(" D:"+shortDate(item.Deadline.DateString())+" "))
	}

	if len(parts) == 0 {
		return ""
	}

	return lipgloss.JoinHorizontal(lipgloss.Top, parts...)
}
