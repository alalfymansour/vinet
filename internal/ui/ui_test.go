package ui

import (
	"strings"
	"testing"

	"github.com/alalfymansour/vinet/internal/db"
	"github.com/charmbracelet/bubbles/table"
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

func TestViewFitsWideTerminal(t *testing.T) {
	database, err := db.InitDBAt(t.TempDir() + "/traffic.db")
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	m := InitialModel(database, 0)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 128, Height: 40})
	lines := strings.Count(updated.(model).View(), "\n") + 1
	if lines > 40 {
		t.Fatalf("rendered %d lines in a 40-line terminal", lines)
	}
}

func TestTableHeaderAlignsWithRows(t *testing.T) {
	m := InitialModel(nil, 0)
	m.table.SetRows([]table.Row{{"codex", "2 B/s", "1 MiB", "2 MiB"}})
	lines := strings.Split(m.table.View(), "\n")
	var header, row string
	for _, line := range lines {
		if strings.Contains(line, "Process") {
			header = line
		}
		if strings.Contains(line, "codex") {
			row = line
		}
	}
	if strings.Index(header, "Process") != strings.Index(row, "codex") {
		t.Fatalf("process header starts at %d, row starts at %d\nheader=%q\nrow=%q", strings.Index(header, "Process"), strings.Index(row, "codex"), header, row)
	}
}
