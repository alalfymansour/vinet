package ui

import (
	"strings"
	"testing"

	"github.com/alalfymansour/vinet/internal/db"
	tea "github.com/charmbracelet/bubbletea"
)

func TestSummaryIncludesCurrentTraffic(t *testing.T) {
	database, err := db.InitDBAt(t.TempDir() + "/traffic.db")
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	if _, err := database.Exec("INSERT INTO traffic (process_name, dest_ip, bytes_sent, bytes_recv) VALUES ('test-process', '127.0.0.1', 10, 20)"); err != nil {
		t.Fatal(err)
	}

	model := InitialModel(database, 0)
	model.updateSummary()
	if model.err != "" {
		t.Fatalf("summary update failed: %s", model.err)
	}
	rows := model.table.Rows()
	if len(rows) != 1 || rows[0][0] != "test-process" {
		t.Fatalf("summary rows = %#v, want test-process", rows)
	}
}

func TestViewFitsSmallTerminal(t *testing.T) {
	database, err := db.InitDBAt(t.TempDir() + "/traffic.db")
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	m := InitialModel(database, 0)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 60, Height: 15})
	lines := strings.Count(updated.(model).View(), "\n") + 1
	if lines > 15 {
		t.Fatalf("rendered %d lines in a 15-line terminal", lines)
	}
}
