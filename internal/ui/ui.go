package ui

import (
	"database/sql"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/table"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/alalfymansour/vinet/internal/db"
)

// Shared styles.
var (
	downStyle = lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "#79740e", Dark: "#b8bb26"})
	upStyle   = lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "#af3a03", Dark: "#fe8019"})
	dimStyle  = lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "#7c6f64", Dark: "#928374"})
	boldStyle = lipgloss.NewStyle().Bold(true)
)

// sortModes cycles with the "s" key.
var sortModes = []string{"Download", "Upload", "Rate"}

type model struct {
	db             *sql.DB
	table          table.Model
	timeRange      string
	timeLabel      string
	viewMode       string
	selected       string
	sortMode       int
	totalDown      int64
	totalUp        int64
	err            string
	health         string
	interfaceIndex int
	filter         string
	searching      bool
	width          int
	height         int
}

type tickMsg time.Time

func tick() tea.Cmd {
	return tea.Tick(2*time.Second, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}

func InitialModel(database *sql.DB, interfaceIndex int) model {
	return model{
		db:             database,
		table:          setupSummaryTable(),
		timeRange:      "start of day",
		timeLabel:      "Today",
		viewMode:       "summary",
		interfaceIndex: interfaceIndex,
		width:          80,
		height:         24,
	}
}

func setupSummaryTable() table.Model {
	return table.New(table.WithColumns(summaryColumns()), table.WithRows([]table.Row{}), table.WithHeight(12))
}

func setupDetailTable() table.Model {
	return table.New(table.WithColumns(detailColumns()), table.WithRows([]table.Row{}), table.WithHeight(12))
}

func summaryColumns() []table.Column {
	return []table.Column{{Title: "Process", Width: 20}, {Title: "Down Rate", Width: 14}, {Title: "Total Down", Width: 22}, {Title: "Total Up", Width: 22}}
}

func compactSummaryColumns() []table.Column {
	return []table.Column{{Title: "Proc", Width: 14}, {Title: "Rate", Width: 10}, {Title: "Down", Width: 16}, {Title: "Up", Width: 16}}
}
func detailColumns() []table.Column {
	return []table.Column{{Title: "Destination IP", Width: 20}, {Title: "Total Down", Width: 22}, {Title: "Total Up", Width: 22}}
}

func compactDetailColumns() []table.Column {
	return []table.Column{{Title: "Destination", Width: 16}, {Title: "Down", Width: 16}, {Title: "Up", Width: 16}}
}

func (m model) Init() tea.Cmd { return tick() }

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
		case "esc":
			if m.viewMode == "detail" {
				m.viewMode = "summary"
				m.selected = ""
				m.table = setupSummaryTable()
				m.resizeTable()
				m.updateSummary()
			}
		}
	}

	var cmd tea.Cmd
	m.table, cmd = m.table.Update(msg)
	return m, cmd
}

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
	// separators, and a footer. Keep the rendered output within the terminal.
	height := m.height - 6
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

func (m *model) updateSummary() {
	m.err = ""
	m.health = "collector: unknown"
	var lastSeen, lastError string
	if err := m.db.QueryRow("SELECT COALESCE(last_seen, ''), COALESCE(last_error, '') FROM collector_state WHERE id=1").Scan(&lastSeen, &lastError); err == nil && lastSeen != "" {
		status := "active"
		if seenAt, parseErr := time.Parse("2006-01-02 15:04:05", lastSeen); parseErr == nil && time.Since(seenAt.UTC()) > 15*time.Second {
			status = "stale"
		}
		m.health = "collector: " + status
		if lastError != "" {
			m.health += " (" + lastError + ")"
		}
	}
	interfaceFilter := ""
	args := []any{m.timeRange}
	if m.interfaceIndex > 0 {
		interfaceFilter = " AND ifindex = ?"
		args = append(args, m.interfaceIndex)
	}
	processFilter := ""
	if m.filter != "" {
		processFilter = " AND process_name LIKE ?"
		args = append(args, "%"+m.filter+"%")
	}
	// Global totals for the header (across ALL processes, not just top 12).
	if err := m.db.QueryRow(`
		SELECT COALESCE(SUM(bytes_recv), 0), COALESCE(SUM(bytes_sent), 0)
		FROM traffic
		WHERE timestamp >= datetime('now', ?)
	`+interfaceFilter+processFilter, args...).Scan(&m.totalDown, &m.totalUp); err != nil {
		m.err = err.Error()
		return
	}

	rows, err := m.db.Query(`
		SELECT process_name, SUM(bytes_recv), SUM(bytes_sent)
		FROM traffic
		WHERE timestamp >= datetime('now', ?)
	`+interfaceFilter+processFilter+`
		GROUP BY process_name
		ORDER BY SUM(bytes_recv) DESC
		LIMIT 12;
	`, args...)
	if err != nil {
		m.err = err.Error()
		return
	}

	type procData struct {
		name     string
		down, up int64
	}
	var data []procData
	for rows.Next() {
		var d procData
		if err := rows.Scan(&d.name, &d.down, &d.up); err != nil {
			m.err = err.Error()
			rows.Close()
			return
		}
		data = append(data, d)
	}
	rows.Close()

	// Live rates (bytes/sec) over the last 30 seconds. The collector normally
	// polls every five seconds, so this remains responsive while tolerating a
	// delayed tick during system load or service startup.
	rateArgs := []any{}
	rateFilter := ""
	if m.interfaceIndex > 0 {
		rateFilter = " AND ifindex = ?"
		rateArgs = append(rateArgs, m.interfaceIndex)
	}
	rateRows, err := m.db.Query(`
		SELECT process_name, SUM(bytes_recv), MAX((julianday('now') - julianday(MIN(timestamp))) * 86400.0, 1)
		FROM traffic
		WHERE timestamp >= datetime('now', '-30 seconds')
	`+rateFilter+`
		GROUP BY process_name;
	`, rateArgs...)
	if err != nil {
		m.err = err.Error()
		return
	}
	liveRates := make(map[string]int64)
	for rateRows.Next() {
		var name string
		var down int64
		var seconds float64
		if err := rateRows.Scan(&name, &down, &seconds); err != nil {
			m.err = err.Error()
			rateRows.Close()
			return
		}
		liveRates[name] = int64(float64(down) / seconds)
	}
	rateRows.Close()

	// Apply the active sort mode.
	switch m.sortMode {
	case 1: // Upload
		sort.Slice(data, func(i, j int) bool { return data[i].up > data[j].up })
	case 2: // Rate
		sort.Slice(data, func(i, j int) bool { return liveRates[data[i].name] > liveRates[data[j].name] })
	default: // Download
		sort.Slice(data, func(i, j int) bool { return data[i].down > data[j].down })
	}

	maxDown, maxUp := int64(1), int64(1)
	for _, d := range data {
		if d.down > maxDown {
			maxDown = d.down
		}
		if d.up > maxUp {
			maxUp = d.up
		}
	}

	var newRows []table.Row
	for _, d := range data {
		newRows = append(newRows, table.Row{
			d.name,
			downStyle.Render(db.FormatBytes(liveRates[d.name]) + "/s"),
			fmt.Sprintf("%s %s", renderBar(d.down, maxDown, 8), downStyle.Render(db.FormatBytes(d.down))),
			fmt.Sprintf("%s %s", renderBar(d.up, maxUp, 8), upStyle.Render(db.FormatBytes(d.up))),
		})
	}
	m.table.SetRows(newRows)
}

func (m *model) updateDetail() {
	m.err = ""
	interfaceFilter := ""
	args := []any{m.selected, m.timeRange}
	if m.interfaceIndex > 0 {
		interfaceFilter = " AND ifindex = ?"
		args = append(args, m.interfaceIndex)
	}
	rows, err := m.db.Query(`
		SELECT COALESCE(dest_ip, ''), dest_port, protocol, ifindex, SUM(bytes_recv), SUM(bytes_sent)
		FROM traffic
		WHERE process_name = ? AND timestamp >= datetime('now', ?)
	`+interfaceFilter+`
		GROUP BY dest_ip, dest_port, protocol, ifindex
		ORDER BY SUM(bytes_recv) + SUM(bytes_sent) DESC
		LIMIT 12;
	`, args...)
	if err != nil {
		m.err = err.Error()
		return
	}

	type ipData struct {
		ip       string
		port     uint16
		protocol uint8
		ifindex  uint32
		down, up int64
	}
	var data []ipData
	for rows.Next() {
		var d ipData
		if err := rows.Scan(&d.ip, &d.port, &d.protocol, &d.ifindex, &d.down, &d.up); err != nil {
			m.err = err.Error()
			rows.Close()
			return
		}
		if d.ip == "0.0.0.0" {
			d.ip = "localhost/listen"
		}
		if d.port != 0 {
			if strings.Contains(d.ip, ":") {
				d.ip = fmt.Sprintf("[%s]:%d/%s", d.ip, d.port, db.ProtocolName(int(d.protocol)))
			} else {
				d.ip = fmt.Sprintf("%s:%d/%s", d.ip, d.port, db.ProtocolName(int(d.protocol)))
			}
		}
		data = append(data, d)
	}
	rows.Close()

	maxDown, maxUp := int64(1), int64(1)
	for _, d := range data {
		if d.down > maxDown {
			maxDown = d.down
		}
		if d.up > maxUp {
			maxUp = d.up
		}
	}

	var newRows []table.Row
	for _, d := range data {
		newRows = append(newRows, table.Row{
			d.ip,
			fmt.Sprintf("%s %s", renderBar(d.down, maxDown, 8), downStyle.Render(db.FormatBytes(d.down))),
			fmt.Sprintf("%s %s", renderBar(d.up, maxUp, 8), upStyle.Render(db.FormatBytes(d.up))),
		})
	}
	m.table.SetRows(newRows)
}

func renderBar(val, max int64, width int) string {
	if max == 0 {
		return strings.Repeat("░", width)
	}
	filled := int(float64(val) / float64(max) * float64(width))
	if filled > width {
		filled = width
	}
	return strings.Repeat("█", filled) + strings.Repeat("░", width-filled)
}

func (m model) View() string {
	baseStyle := lipgloss.NewStyle().
		BorderStyle(lipgloss.NormalBorder()).
		BorderForeground(lipgloss.Color("240"))
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
			header += "    " + dimStyle.Render(m.health)
			footer = dimStyle.Render("d/D day  w/W week  m/M month  •  s sort  •  / search  •  h back  •  l details  •  j/k select  •  q quit")
		} else {
			header = boldStyle.Render(fmt.Sprintf("ViNet · %s · ↓ %s · ↑ %s", m.timeLabel, db.FormatBytes(m.totalDown), db.FormatBytes(m.totalUp)))
			footer = dimStyle.Render("d/D day  w/W week  m/M month • s sort • / search • h back • l details • j/k select • q quit")
		}
		if m.searching {
			footer = dimStyle.Render("Search: " + m.filter + "_  enter apply • esc close")
		}
	} else {
		header = boldStyle.Render(fmt.Sprintf("ViNet · IPs for '%s' (%s)", m.selected, m.timeLabel))
		footer = dimStyle.Render("h back  •  q quit")
	}

	if m.err != "" {
		return fmt.Sprintf("%s\n\n%s\n\n%s", header, dimStyle.Render("Database error: "+m.err), footer)
	}
	tableView := m.table.View()
	if m.width > 120 {
		tableView = lipgloss.NewStyle().Width(m.width - 2).Align(lipgloss.Center).Render(tableView)
	}
	return fmt.Sprintf("%s\n\n%s\n\n%s", header, baseStyle.Render(tableView), footer)
}
