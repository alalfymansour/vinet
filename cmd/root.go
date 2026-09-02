package cmd

import (
	"database/sql"
	"fmt"
	"os"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/cobra"

	"github.com/alalfymansour/vinet/internal/db"
	"github.com/alalfymansour/vinet/internal/ui"
)

var (
	timeFlag    string
	dayFlag     bool
	monthFlag   bool
	weekFlag    bool
	liveFlag    bool
	statusFlag  bool
	versionFlag bool
)

// timeFilters maps user-facing ranges to SQLite datetime modifiers.
var timeFilters = map[string]string{
	"today": "start of day",
	"24h":   "-1 day",
	"week":  "-7 days",
	"7d":    "-7 days",
	"month": "start of month",
	"30d":   "-30 days",
}

var timeLabels = map[string]string{
	"today": "today",
	"24h":   "last 24 hours",
	"week":  "this week",
	"7d":    "last 7 days",
	"month": "this month",
	"30d":   "last 30 days",
}

var downStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("42")) // green
var upStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("214"))  // orange
var dimStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
var boldStyle = lipgloss.NewStyle().Bold(true)

var rootCmd = &cobra.Command{
	Use:   "vinet [process]",
	Short: "Track historical internet usage per process (TUI + CLI).",
	Long: `ViNet tracks per-process network usage with eBPF.

Run without arguments to open the interactive TUI dashboard, pass a
process name for a quick usage query, or use --live for a lightweight
live view straight in your terminal.

Examples:
  vinet                 Open the dashboard
  vinet -w firefox      Show Firefox usage this week
  vinet -l              Show live traffic
  vinet -s              Show daemon status
  vinet export backup.json
  vinet import backup.json`,
	Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := applyTimeShortcut(cmd); err != nil {
			return err
		}
		if versionFlag {
			fmt.Printf("ViNet %s\n", Version)
			return nil
		}
		if statusFlag {
			return runStatus()
		}
		filter, ok := timeFilters[timeFlag]
		if !ok {
			return fmt.Errorf("invalid --time value %q (want: today, 24h, week, 7d, month, 30d)", timeFlag)
		}

		database, err := openDatabase()
		if err != nil {
			return fmt.Errorf("initializing DB: %w", err)
		}
		defer database.Close()

		// MODE 1: live CLI view (optionally scoped to one process)
		if liveFlag {
			runLive(database, args)
			return nil
		}

		// MODE 2: quick CLI query for one process
		if len(args) > 0 {
			return runQuery(database, args[0], filter)
		}

		// MODE 3: TUI dashboard
		p := tea.NewProgram(ui.InitialModel(database, 0), tea.WithAltScreen())
		if _, err := p.Run(); err != nil {
			return fmt.Errorf("running TUI: %w", err)
		}
		return nil
	},
}

func applyTimeShortcut(cmd *cobra.Command) error {
	shortcuts := 0
	if dayFlag {
		shortcuts++
	}
	if monthFlag {
		shortcuts++
	}
	if weekFlag {
		shortcuts++
	}
	if shortcuts > 1 {
		return fmt.Errorf("choose only one of -d, -m, or -w")
	}
	if shortcuts == 0 {
		return nil
	}
	if cmd.Flags().Changed("time") {
		return fmt.Errorf("do not combine -d, -m, or -w with --time")
	}
	switch {
	case dayFlag:
		timeFlag = "today"
	case monthFlag:
		timeFlag = "month"
	case weekFlag:
		timeFlag = "week"
	}
	return nil
}

func runQuery(database *sql.DB, processName, filter string) error {
	var down, up int64
	args := []any{processName, filter}
	err := database.QueryRow(fmt.Sprintf(`
		SELECT COALESCE(SUM(bytes_recv), 0), COALESCE(SUM(bytes_sent), 0)
		FROM traffic
		WHERE process_name = ? AND timestamp >= datetime('now', ?)
	`), args...).Scan(&down, &up)
	if err != nil {
		return err
	}

	if down == 0 && up == 0 {
		fmt.Printf("No traffic recorded for '%s' %s.\n", processName, timeLabels[timeFlag])
		return nil
	}

	fmt.Printf("ViNet - Usage for '%s' (%s):\n", processName, timeLabels[timeFlag])
	fmt.Printf("  ↓ Download: %s\n", downStyle.Render(db.FormatBytes(down)))
	fmt.Printf("  ↑ Upload:   %s\n", upStyle.Render(db.FormatBytes(up)))
	return nil
}

// runLive prints a continuously refreshing, non-TUI view of current traffic
// rates. If a process name was given, it is scoped to that process only.
func runLive(database *sql.DB, args []string) {
	processFilter := ""
	var queryArgs []any
	if len(args) > 0 {
		processFilter = " AND process_name = ?"
		queryArgs = append(queryArgs, args[0])
	}
	query := fmt.Sprintf(`
		SELECT process_name, COALESCE(SUM(bytes_recv), 0), COALESCE(SUM(bytes_sent), 0), MAX((julianday('now') - julianday(MIN(timestamp))) * 86400.0, 1)
		FROM traffic
		WHERE timestamp >= datetime('now', '-30 seconds')%s
		GROUP BY process_name
		ORDER BY SUM(bytes_recv) + SUM(bytes_sent) DESC
		LIMIT 10;
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

		fmt.Printf("  %-18s %16s %16s\n", "PROCESS", "DOWN", "UP")
		for _, r := range data {
			fmt.Printf("  %-18s %16s %16s\n",
				r.name,
				downStyle.Render(db.FormatBytes(int64(float64(r.down)/r.seconds))+"/s"),
				upStyle.Render(db.FormatBytes(int64(float64(r.up)/r.seconds))+"/s"),
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

func Execute() {
	rootCmd.Flags().StringVarP(&timeFlag, "time", "t", "today", "Time range: today, 24h, week, 7d, month, 30d")
	rootCmd.Flags().BoolVarP(&dayFlag, "day", "d", false, "Today")
	rootCmd.Flags().BoolVarP(&monthFlag, "month", "m", false, "This month")
	rootCmd.Flags().BoolVarP(&weekFlag, "week", "w", false, "This week")
	rootCmd.Flags().BoolVarP(&liveFlag, "live", "l", false, "Live view of current traffic (no TUI)")
	rootCmd.Flags().BoolVarP(&statusFlag, "status", "s", false, "Show daemon and database status")
	rootCmd.Flags().BoolVarP(&versionFlag, "version", "v", false, "Show version")
	rootCmd.SilenceUsage = true
	rootCmd.SilenceErrors = true
	rootCmd.AddCommand(daemonCmd)
	rootCmd.AddCommand(exportCmd)
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "Error:", err)
		os.Exit(1)
	}
}

func openDatabase() (*sql.DB, error) {
	return db.InitDB()
}
