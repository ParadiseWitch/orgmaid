package ui

import (
	"github.com/charmbracelet/lipgloss"
)

// ExpandableCell renders a cell that can expand from a compact size to a larger
// size when focused. The expansion overlays the content area rather than pushing
// other columns, so the row layout stays stable.
type ExpandableCell struct {
	compactWidth  int
	expandedWidth int
}

// NewExpandableCell creates a cell with the given compact and expanded widths.
func NewExpandableCell(compactWidth, expandedWidth int) ExpandableCell {
	return ExpandableCell{
		compactWidth:  compactWidth,
		expandedWidth: expandedWidth,
	}
}

// CompactWidth returns the width when not expanded.
func (e ExpandableCell) CompactWidth() int { return e.compactWidth }

// ExpandedWidth returns the width when expanded.
func (e ExpandableCell) ExpandedWidth() int { return e.expandedWidth }

// RenderCompact renders the compact view with space separators.
// The text is centered with spaces on both sides (e.g., " T " or " S ").
func (e ExpandableCell) RenderCompact(text string, fgColor, bgColor lipgloss.TerminalColor, ground lipgloss.TerminalColor) string {
	styled := lipgloss.NewStyle().
		Foreground(fgColor).
		Background(bgColor).
		Render(text)
	return cell(styled, e.compactWidth, lipgloss.Center, fg(pal.Text), ground)
}

// RenderExpanded renders the expanded view with space separators.
// The text should already include leading/trailing spaces (e.g., " TODO " or " S: 2026-09-21 09:30 ").
func (e ExpandableCell) RenderExpanded(text string, fgColor, bgColor lipgloss.TerminalColor) string {
	return cell(text, e.expandedWidth, lipgloss.Left, fgColor, bgColor)
}

// RenderEmptyCompact renders an empty compact cell with spaces.
func (e ExpandableCell) RenderEmptyCompact(ground lipgloss.TerminalColor) string {
	spaces := ""
	for i := 0; i < e.compactWidth; i++ {
		spaces += " "
	}
	return cell(spaces, e.compactWidth, lipgloss.Center, fg(pal.Dim), ground)
}

// RenderEmptyExpanded renders an empty expanded cell with spaces.
func (e ExpandableCell) RenderEmptyExpanded() string {
	spaces := ""
	for i := 0; i < e.expandedWidth; i++ {
		spaces += " "
	}
	return cell(spaces, e.expandedWidth, lipgloss.Center, fg(pal.Dim), transparent)
}

// ExpandDirection indicates whether expansion should go forward (right) or backward (left).
type ExpandDirection int

const (
	ExpandForward  ExpandDirection = iota // Expand to the right (default)
	ExpandBackward                        // Expand to the left (when hitting right edge)
)

// CalculateExpandPosition determines the x-position for overlay and the direction
// of expansion based on available screen width.
func CalculateExpandPosition(baseX, expandedWidth, screenWidth int) (x int, direction ExpandDirection) {
	// Try forward expansion first
	if baseX+expandedWidth <= screenWidth {
		return baseX, ExpandForward
	}
	// Fall back to backward expansion
	x = baseX - (expandedWidth - baseX)
	if x < 0 {
		x = 0
	}
	return x, ExpandBackward
}
