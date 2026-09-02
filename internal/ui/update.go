package ui

import (
	tea "github.com/charmbracelet/bubbletea"
)

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.resizeTable()
		return m, nil
	case tickMsg:
		if m.viewMode == "summary" {
			m.updateSummary()
		} else {
			m.updateDetail()
		}
		return m, tick()

	case tea.KeyMsg:
		if m.searching {
			switch msg.String() {
			case "enter", "esc":
				m.searching = false
				m.updateSummary()
				return m, nil
			case "ctrl+c":
				return m, tea.Quit
			case "backspace", "delete":
				if len(m.filter) > 0 {
					m.filter = m.filter[:len(m.filter)-1]
				}
				m.updateSummary()
				return m, nil
			}
			if len(msg.Runes) > 0 {
				m.filter += string(msg.Runes)
				m.updateSummary()
			}
			return m, nil
		}
		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		case "?":
			m.showHelp = true
			return m, nil
		case "esc":
			if m.showHelp {
				m.showHelp = false
				return m, nil
			}
			if m.viewMode == "detail" {
				m.viewMode = "summary"
				m.selected = ""
				m.table = setupSummaryTable()
				m.resizeTable()
				m.updateSummary()
			}
			return m, nil
		case "j", "down":
			if rows := m.table.Rows(); len(rows) > 0 && m.table.Cursor() < len(rows)-1 {
				m.table.SetCursor(m.table.Cursor() + 1)
			}
			return m, nil
		case "k", "up":
			if m.table.Cursor() > 0 {
				m.table.SetCursor(m.table.Cursor() - 1)
			}
			return m, nil
		case "h":
			if m.viewMode == "detail" {
				m.viewMode = "summary"
				m.selected = ""
				m.table = setupSummaryTable()
				m.resizeTable()
				m.updateSummary()
			}
			return m, nil
		case "d":
			if m.viewMode == "summary" {
				m.timeRange, m.timeLabel = "start of day", "Today"
				m.updateSummary()
			}
		case "D":
			if m.viewMode == "summary" {
				m.timeRange, m.timeLabel = "-1 day", "Last 24 Hours"
				m.updateSummary()
			}
		case "w":
			if m.viewMode == "summary" {
				m.timeRange, m.timeLabel = "-7 days", "This Week"
				m.updateSummary()
			}
		case "W":
			if m.viewMode == "summary" {
				m.timeRange, m.timeLabel = "-7 days", "Last 7 Days"
				m.updateSummary()
			}
		case "m":
			if m.viewMode == "summary" {
				m.timeRange, m.timeLabel = "start of month", "This Month"
				m.updateSummary()
			}
		case "M":
			if m.viewMode == "summary" {
				m.timeRange, m.timeLabel = "-30 days", "Last 30 Days"
				m.updateSummary()
			}
		case "s":
			if m.viewMode == "summary" {
				m.sortMode = (m.sortMode + 1) % len(sortModes)
				m.updateSummary()
			}
		case "/":
			if m.viewMode == "summary" {
				m.searching = true
				return m, nil
			}
		case "l", "enter":
			if m.viewMode == "summary" {
				row := m.table.SelectedRow()
				if len(row) > 0 {
					m.selected = row[0]
					m.viewMode = "detail"
					m.table = setupDetailTable()
					m.resizeTable()
					m.updateDetail()
				}
			}
		}
	}

	var cmd tea.Cmd
	m.table, cmd = m.table.Update(msg)
	return m, cmd
}
