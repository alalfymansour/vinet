package ui

import (
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/mattn/go-runewidth"

	"github.com/alalfymansour/vinet/internal/db"
)

// RunQuery prints a one-shot usage summary for a single process. The filter
// is a SQLite datetime modifier and label is its human-facing description.
func RunQuery(database *sql.DB, processName, filter, label string) error {
	var down, up int64
	args := []any{processName, filter}
	err := database.QueryRow(`
		SELECT COALESCE(SUM(bytes_recv), 0), COALESCE(SUM(bytes_sent), 0)
		FROM traffic
		WHERE process_name = ? AND timestamp >= datetime('now', ?)
	`, args...).Scan(&down, &up)
	if err != nil {
		return err
	}

	if down == 0 && up == 0 {
		fmt.Printf("No traffic recorded for '%s' %s.\n", processName, label)
		return nil
	}

	fmt.Printf("ViNet - Usage for '%s' (%s):\n", processName, label)
	fmt.Printf("  ↓ Download: %s\n", downStyle.Render(db.FormatBytes(down)))
	fmt.Printf("  ↑ Upload:   %s\n", upStyle.Render(db.FormatBytes(up)))
	return nil
}

// Live prints a continuously refreshing, non-TUI view of current traffic
// rates. If a process name is given, it is scoped to that process only.
func Live(database *sql.DB, process string) {
	processFilter := ""
	var queryArgs []any
	if process != "" {
		processFilter = " AND process_name = ?"
		queryArgs = append(queryArgs, process)
	}
	query := fmt.Sprintf(`
		SELECT process_name, COALESCE(SUM(bytes_recv), 0), COALESCE(SUM(bytes_sent), 0), MAX((julianday('now') - julianday(MIN(timestamp))) * 86400.0, 1)
		FROM traffic
		WHERE timestamp >= datetime('now', '-30 seconds')%s
		GROUP BY process_name
		ORDER BY SUM(bytes_recv) + SUM(bytes_sent) DESC;
	`, processFilter)

	type liveRow struct {
		name     string
		down, up int64
		seconds  float64
	}

	render := func() {
		rows, err := database.Query(query, queryArgs...)
		if err != nil {
			fmt.Println("Query error:", err)
			return
		}
		defer rows.Close()

		var data []liveRow
		var totalDown, totalUp int64
		for rows.Next() {
			var r liveRow
			if err := rows.Scan(&r.name, &r.down, &r.up, &r.seconds); err != nil {
				continue
			}
			data = append(data, r)
			totalDown += int64(float64(r.down) / r.seconds)
			totalUp += int64(float64(r.up) / r.seconds)
		}

		// Clear screen and draw the frame.
		fmt.Print("\033[H\033[2J")
		fmt.Println(boldStyle.Render("ViNet — Live"), dimStyle.Render(time.Now().Format("15:04:05")))

		if len(data) == 0 {
			fmt.Println(dimStyle.Render("No traffic in the last 30 seconds. Is the daemon running?"))
			return
		}

		fmt.Printf("  %-24s %16s %16s\n", "PROCESS", "DOWN RATE", "UP RATE")
		for _, r := range data {
			down := db.FormatBytes(int64(float64(r.down)/r.seconds)) + "/s"
			up := db.FormatBytes(int64(float64(r.up)/r.seconds)) + "/s"
			fmt.Printf("  %s %s %s\n",
				liveText(r.name, 24, false, lipgloss.NewStyle()),
				liveText(down, 16, true, downStyle),
				liveText(up, 16, true, upStyle),
			)
		}
		fmt.Println()
		fmt.Printf("  %s %s   %s %s\n",
			dimStyle.Render("↓ Total:"),
			downStyle.Render(db.FormatBytes(totalDown)+"/s"),
			dimStyle.Render("↑ Total:"),
			upStyle.Render(db.FormatBytes(totalUp)+"/s"),
		)
	}

	render()
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	for range ticker.C {
		render()
	}
}

func liveText(value string, width int, rightAlign bool, style lipgloss.Style) string {
	value = runewidth.Truncate(value, width, "…")
	padding := width - runewidth.StringWidth(value)
	if rightAlign {
		value = strings.Repeat(" ", padding) + value
	} else {
		value += strings.Repeat(" ", padding)
	}
	if style.GetForeground() != nil {
		return style.Render(value)
	}
	return value
}
