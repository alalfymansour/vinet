package ui

import (
	"github.com/charmbracelet/bubbles/table"
	"github.com/charmbracelet/lipgloss"
)

// Shared styles.
var (
	downStyle = lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "#79740e", Dark: "#b8bb26"})
	upStyle   = lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "#af3a03", Dark: "#fe8019"})
	dimStyle  = lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "#7c6f64", Dark: "#928374"})
	boldStyle = lipgloss.NewStyle().Bold(true)
	greenDot  = lipgloss.NewStyle().Foreground(lipgloss.Color("#b8bb26"))
	redDot    = lipgloss.NewStyle().Foreground(lipgloss.Color("#fb4934"))
	// Keep header and cell content at exactly the same column origin. Padding
	// here is applied differently by the table widget to headers and rows.
	tableHeader = lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "#665c54", Dark: "#d5c4a1"}).Align(lipgloss.Left)
	// Leave the foreground unset so the selected-row style can color the
	// process currently under the cursor.
	tableCell     = lipgloss.NewStyle()
	tableSelected = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.AdaptiveColor{Light: "#7c4f00", Dark: "#fabd2f"}).Background(lipgloss.AdaptiveColor{Light: "#fbf1c7", Dark: "#3c3836"})
)

// sortModes cycles with the "s" key.
var sortModes = []string{"Download", "Upload", "Rate"}

func dataTableStyles() table.Styles {
	return table.Styles{Header: tableHeader, Cell: tableCell, Selected: tableSelected}
}
