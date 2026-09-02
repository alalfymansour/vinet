package ui

import (
	"github.com/charmbracelet/bubbles/table"
)

func (m *model) resizeTable() {
	if m.width > 0 {
		layoutWidth := m.width
		if layoutWidth > 120 {
			layoutWidth = 120
		}
		m.table.SetWidth(layoutWidth)
		if m.viewMode == "summary" {
			columns := summaryColumns()
			minimums := []int{12, 10, 18, 18}
			if m.width < 100 {
				columns, minimums = compactSummaryColumns(), []int{8, 7, 8, 8}
			}
			m.table.SetColumns(fitColumns(layoutWidth, columns, minimums))
		} else {
			columns := detailColumns()
			minimums := []int{16, 18, 18}
			if m.width < 100 {
				columns, minimums = compactDetailColumns(), []int{8, 8, 8}
			}
			m.table.SetColumns(fitColumns(layoutWidth, columns, minimums))
		}
	}
	// table.SetHeight includes its header; the view also has a header, two
	// separators, a footer, the table's two border rows, and renderer padding.
	// Keep the complete view below the terminal height; exceeding it makes
	// Bubble Tea continuously scroll the terminal instead of repainting it.
	height := m.height - 12
	if height < 3 {
		height = 3
	}
	m.table.SetHeight(height)
}

func fitColumns(width int, columns []table.Column, minimums []int) []table.Column {
	if width <= 2 {
		return columns
	}
	target := width - 2
	total := 0
	for _, column := range columns {
		total += column.Width
	}
	if target > total {
		extra := target - total
		remaining := extra
		for i := range columns {
			addition := extra * columns[i].Width / total
			if i == len(columns)-1 {
				addition = remaining
			}
			columns[i].Width += addition
			remaining -= addition
		}
		return columns
	}
	minimumTotal := 0
	for _, minimum := range minimums {
		minimumTotal += minimum
	}
	if target < minimumTotal {
		for i := range columns {
			columns[i].Width = target / len(columns)
		}
		for i := 0; i < target%len(columns); i++ {
			columns[i].Width++
		}
		return columns
	}
	for i := range columns {
		if total <= target {
			break
		}
		remove := columns[i].Width - minimums[i]
		if remove > total-target {
			remove = total - target
		}
		if remove > 0 {
			columns[i].Width -= remove
			total -= remove
		}
	}
	return columns
}
