package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/mattn/go-runewidth"

	"github.com/alalfymansour/vinet/internal/db"
)

func (m model) View() string {
	baseStyle := lipgloss.NewStyle().
		BorderStyle(lipgloss.NormalBorder()).
		BorderForeground(lipgloss.Color("#b57614"))
	if m.width > 2 {
		baseStyle = baseStyle.Width(m.width - 2)
	}

	var header, footer string
	if m.viewMode == "summary" {
		if m.width > 100 {
			header = fmt.Sprintf("%s    %s %s    %s %s    %s",
				boldStyle.Render(fmt.Sprintf("ViNet · %s Usage", m.timeLabel)),
				dimStyle.Render("↓ Total:"), downStyle.Render(db.FormatBytes(m.totalDown)),
				dimStyle.Render("↑ Total:"), upStyle.Render(db.FormatBytes(m.totalUp)),
				dimStyle.Render(fmt.Sprintf("Sort: %s", sortModes[m.sortMode])),
			)
			header += "    " + collectorDot(m.health)
			footer = dimStyle.Render("d/D day  w/W week  m/M month  •  s sort  •  / search  •  h back  •  l details  •  j/k select  •  ? help  •  q quit")
		} else {
			header = boldStyle.Render(fmt.Sprintf("ViNet · %s · ↓ %s · ↑ %s", m.timeLabel, db.FormatBytes(m.totalDown), db.FormatBytes(m.totalUp)))
			footer = dimStyle.Render("d/D day  w/W week  m/M month • s sort • / search • h back • l details • j/k select • ? help • q quit")
		}
		if m.searching {
			footer = dimStyle.Render("Search: " + m.filter + "_  enter apply • esc close")
		}
	} else {
		header = boldStyle.Render(fmt.Sprintf("ViNet · IPs for '%s' (%s)", m.selected, m.timeLabel))
		footer = dimStyle.Render("h back  •  ? help  •  q quit")
	}

	if m.err != "" {
		return fmt.Sprintf("%s\n\n%s\n\n%s", header, dimStyle.Render("Database error: "+m.err), footer)
	}
	if m.showHelp {
		return fmt.Sprintf("%s\n\n%s\n\n%s", header, renderHelp(), dimStyle.Render("press ? or esc to close"))
	}
	tableView := renderTable(m)
	if m.width > 120 {
		tableView = lipgloss.NewStyle().Width(m.width - 2).Align(lipgloss.Center).Render(tableView)
	}
	return fmt.Sprintf("%s\n%s\n%s", header, baseStyle.Render(tableView), footer)
}

func renderTable(m model) string {
	rows := m.table.Rows()
	columns := []int{24, 20, 20, 18}
	titles := []string{"Process", "Total Down", "Total Up", "Down Rate"}
	if m.viewMode == "detail" {
		columns = []int{28, 24, 24}
		titles = []string{"Destination / Protocol", "Total Down", "Total Up"}
	}
	maxWidth := m.width - 6
	if maxWidth > 116 {
		maxWidth = 116
	}
	if maxWidth < 50 {
		maxWidth = 50
	}
	if m.width < 100 {
		if m.viewMode == "summary" {
			columns = []int{16, 15, 15, 12}
		} else {
			columns = []int{20, 15, 15}
		}
	}
	columns = fitRenderColumns(columns, maxWidth)

	lines := []string{renderTableLine(titles, columns, false)}
	visible := m.height - 15
	if visible < 1 {
		visible = 1
	}
	start := 0
	if m.table.Cursor() >= visible {
		start = m.table.Cursor() - visible + 1
	}
	if start > len(rows)-visible {
		start = len(rows) - visible
	}
	if start < 0 {
		start = 0
	}
	end := start + visible
	if end > len(rows) {
		end = len(rows)
	}
	for i := start; i < end; i++ {
		line := renderTableLine(rows[i], columns, true)
		if i == m.table.Cursor() {
			line = tableSelected.Render(line)
		}
		lines = append(lines, line)
	}
	return strings.Join(lines, "\n")
}

func fitRenderColumns(columns []int, width int) []int {
	total := 0
	for _, n := range columns {
		total += n
	}
	if total >= width {
		return columns
	}
	columns[len(columns)-1] += width - total
	return columns
}

func renderTableLine(values []string, widths []int, header bool) string {
	parts := make([]string, len(widths))
	for i, width := range widths {
		value := ""
		if i < len(values) {
			value = values[i]
		}
		value = runewidth.Truncate(value, width, "…")
		if i == 0 {
			parts[i] = value + strings.Repeat(" ", width-runewidth.StringWidth(value))
		} else {
			parts[i] = strings.Repeat(" ", width-runewidth.StringWidth(value)) + value
		}
	}
	line := strings.Join(parts, "  ")
	if header {
		return tableHeader.Render(line)
	}
	return tableCell.Render(line)
}

func renderHelp() string {
	return strings.Join([]string{
		boldStyle.Render("ViNet keyboard help"),
		"",
		"d / D   today / last 24 hours",
		"w / W   this week / last 7 days",
		"m / M   this month / last 30 days",
		"s       sort by download, upload, or rate",
		"j / k   move selection and scroll",
		"enter   open process details",
		"/       search processes",
		"h       return from details",
		"?       show this help",
		"q       quit",
	}, "\n")
}

func collectorDot(health string) string {
	if health == "collector: active" {
		return greenDot.Render("●")
	}
	return redDot.Render("●")
}
