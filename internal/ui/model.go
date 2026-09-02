package ui

import (
	"database/sql"
	"time"

	"github.com/charmbracelet/bubbles/table"
	tea "github.com/charmbracelet/bubbletea"
)

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
	showHelp       bool
}

type tickMsg time.Time

func tick() tea.Cmd {
	return tea.Tick(2*time.Second, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}

func InitialModel(database *sql.DB, interfaceIndex int) model {
	m := model{
		db:             database,
		table:          setupSummaryTable(),
		timeRange:      "start of day",
		timeLabel:      "Today",
		viewMode:       "summary",
		interfaceIndex: interfaceIndex,
		width:          80,
		height:         24,
	}
	// Populate the first frame before Bubble Tea starts. Without this, the
	// dashboard stays empty until the first refresh tick fires.
	if database != nil {
		m.updateSummary()
	}
	return m
}

func setupSummaryTable() table.Model {
	t := table.New(table.WithColumns(summaryColumns()), table.WithRows([]table.Row{}), table.WithHeight(12))
	t.SetStyles(dataTableStyles())
	return t
}

func setupDetailTable() table.Model {
	t := table.New(table.WithColumns(detailColumns()), table.WithRows([]table.Row{}), table.WithHeight(12))
	t.SetStyles(dataTableStyles())
	return t
}

func summaryColumns() []table.Column {
	return []table.Column{{Title: "Process", Width: 20}, {Title: "Total Down", Width: 22}, {Title: "Total Up", Width: 22}, {Title: "Down Rate", Width: 14}}
}

func compactSummaryColumns() []table.Column {
	return []table.Column{{Title: "Proc", Width: 14}, {Title: "Down", Width: 16}, {Title: "Up", Width: 16}, {Title: "Rate", Width: 10}}
}

func detailColumns() []table.Column {
	return []table.Column{{Title: "Destination IP", Width: 20}, {Title: "Total Down", Width: 22}, {Title: "Total Up", Width: 22}}
}

func compactDetailColumns() []table.Column {
	return []table.Column{{Title: "Destination", Width: 16}, {Title: "Down", Width: 16}, {Title: "Up", Width: 16}}
}

func (m model) Init() tea.Cmd { return tick() }
