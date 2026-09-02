package cmd

import (
	"database/sql"
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"
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

// Version is replaced during release builds with -ldflags
// (-X github.com/alalfymansour/vinet/cmd.Version=<tag>).
var Version = "dev"

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
		filter, ok := db.TimeFilters[timeFlag]
		if !ok {
			return fmt.Errorf("invalid --time value %q (want: today, 24h, week, 7d, month, 30d)", timeFlag)
		}

		database, err := openDatabase()
		if err != nil {
			return fmt.Errorf("initializing DB: %w", err)
		}
		defer database.Close()

		process := ""
		if len(args) > 0 {
			process = args[0]
		}

		// MODE 1: live CLI view (optionally scoped to one process)
		if liveFlag {
			ui.Live(database, process)
			return nil
		}

		// MODE 2: quick CLI query for one process
		if process != "" {
			return ui.RunQuery(database, process, filter, db.TimeLabels[timeFlag])
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
