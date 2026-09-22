package ui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"orgmaid/internal/config"
)

// Column widths of a log row, in terminal cells. The columns sit flush against
// one another and are told apart by colour rather than by gaps.
const (
	colMark     = 2 // selection marker plus the space after it
	colIndex    = 3
	colTodo     = 3 // " T " or " D " compact, expands when focused
	colSched    = 3 // " S " compact, expands when focused
	colDead     = 3 // " D " compact, expands when focused
	colTime     = 8
	colDuration = 8
	rowMargin   = 1

	// fixedWidth is every cell left of the content column, marker included.
	// This is the minimum width when nothing is expanded.
	fixedWidth = colMark + colIndex + colTodo + colTime*2 + colDuration + colSched + colDead
)

// expandedWidth is how wide a column gets when focused. The focused column
// overlaps the content area to show its full detail.
const (
	expandedTodo = 6  // " TODO " or " DONE " (with space separators)
	expandedDate = 21 // " S: 2026-09-21 09:30 " or " D: 2026-09-21 09:30 " (with space separators)
)

// expandableCells are reusable components for columns that can expand.
var (
	todoExpandable  = NewExpandableCell(colTodo, expandedTodo)
	schedExpandable = NewExpandableCell(colSched, expandedDate)
	deadExpandable  = NewExpandableCell(colDead, expandedDate)
)

// pal is the live palette: the shipped defaults until Apply reads the user's
// config file, which happens before the first frame is drawn.
var pal = config.Default()

// transparent is the terminal's own background: the program lays no canvas of
// its own any more, so whatever the user's terminal theme is shows through.
// Where a stretch of UI still needs a ground of its own — the selected row, the
// field under the cursor, the status bar — bg names the palette entry for it.
var transparent lipgloss.TerminalColor = lipgloss.NoColor{}

// bg and fg name the two roles a palette entry can play, so a call site reads
// as a sentence: cell(text, width, align, fg(pal.Ink), bg(pal.Index)).
func bg(s string) lipgloss.Color { return lipgloss.Color(s) }
func fg(s string) lipgloss.Color { return lipgloss.Color(s) }

var (
	titleStyle  lipgloss.Style
	dimStyle    lipgloss.Style
	warnStyle   lipgloss.Style
	statusStyle lipgloss.Style
)

func init() { buildStyles() }

// buildStyles rebuilds the styles that carry a palette entry, so a config file
// reaches every corner of the UI and not just the cells built per frame.
func buildStyles() {
	titleStyle = lipgloss.NewStyle().Bold(true).Foreground(fg(pal.Title))
	dimStyle = lipgloss.NewStyle().Foreground(fg(pal.Dim))
	warnStyle = lipgloss.NewStyle().Foreground(fg(pal.Warn))
	statusStyle = lipgloss.NewStyle().Foreground(fg(pal.Dim))
}

// apply takes over the colours from the config file.
func apply(c config.Colors) {
	pal = c
	buildStyles()
}

// on paints plain text; it once carried the program ground and is now the
// identity, kept so the padding call sites stay readable.
func on(s string) string { return s }

// fit truncates to width so lipgloss pads the cell instead of wrapping it onto
// a second line and breaking the row.
func fit(s string, width int) string {
	if width <= 0 {
		return ""
	}
	return ansi.Truncate(s, width, "")
}

// cut clips an already-styled line to width. MaxWidth cannot stand in: it wraps
// onto a second line, which would throw off the list height on a terminal too
// narrow for the row. The reset it appends when it does cut keeps a colour band
// from bleeding past the edge of the screen.
func cut(s string, width int) string {
	if width <= 0 {
		return ""
	}
	return ansi.Truncate(s, width, "\x1b[0m")
}

// cell renders one fixed-width colour block.
func cell(text string, width int, align lipgloss.Position, col, ground lipgloss.TerminalColor) string {
	return lipgloss.NewStyle().
		Width(width).
		Align(align).
		Foreground(col).
		Background(ground).
		Render(fit(text, width))
}

// run paints one stretch of text with no padding of its own. A column is built
// from runs so the half of it the cursor stands on can be picked out while the
// rest keeps the column's own background.
func run(text string, col, ground lipgloss.TerminalColor) string {
	return lipgloss.NewStyle().Foreground(col).Background(ground).Render(text)
}

// choose picks a colour by condition, which keeps a run's two possible grounds
// legible where they are used.
func choose(active bool, yes, no lipgloss.TerminalColor) lipgloss.TerminalColor {
	if active {
		return yes
	}
	return no
}

// overlay paints overlay on top of base starting at column x.
// Both strings are single-line. The overlay replaces characters in base,
// preserving ANSI escapes by working on visible cells.
func overlay(base, overlayStr string, x int) string {
	if x < 0 || overlayStr == "" {
		return base
	}
	baseWidth := lipgloss.Width(base)
	overlayWidth := lipgloss.Width(overlayStr)

	// If overlay starts past the base, just pad and append
	if x >= baseWidth {
		return base + strings.Repeat(" ", x-baseWidth) + overlayStr
	}

	// Split base into before, overlaid, and after sections
	before := ansi.Truncate(base, x, "\x1b[0m")
	rest := ansi.TruncateLeft(base, x, "")

	// The overlay replaces the next overlayWidth cells of rest
	after := ansi.TruncateLeft(rest, overlayWidth, "")

	return before + overlayStr + after
}

// divider is a hairline rule between the page regions, drawn in the border
// colour rather than as a band of background. It is clipped to width because
// U+2500 counts as double width in some CJK terminals: a long rule should lose
// its tail rather than wrap the whole frame onto the next line.
func divider(width int) string {
	if width <= 0 {
		return ""
	}
	rule := lipgloss.NewStyle().
		Foreground(fg(pal.Divider)).
		Render(strings.Repeat("─", width))
	return cut(rule, width)
}

// canvas evens the frame out to the full width and height. The program paints
// no background of its own, so the padding is plain space and the terminal's
// own colours show through; the padding is still laid line by line because a
// vertical join pads a ragged edge with lines that would drift out of step.
func canvas(s string, width, height int) string {
	if width <= 0 || height <= 0 {
		return s
	}

	lines := strings.Split(s, "\n")
	for len(lines) < height {
		lines = append(lines, "")
	}
	for i, line := range lines {
		line = cut(strings.TrimRight(line, " "), width)
		if gap := width - lipgloss.Width(line); gap > 0 {
			line += on(strings.Repeat(" ", gap))
		}
		lines[i] = line
	}
	return strings.Join(lines, "\n")
}

// spread lays left and right text at the two ends of a width-sized line.
func spread(width int, left, right string) string {
	gap := width - lipgloss.Width(left) - lipgloss.Width(right)
	if gap < 1 {
		return fit(left, width)
	}
	return left + on(strings.Repeat(" ", gap)) + right
}

func clamp(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

// wrap folds v into [0, mod), so a spinner can run past either end.
func wrap(v, mod int) int {
	return ((v % mod) + mod) % mod
}
