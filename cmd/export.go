package cmd

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/alalfymansour/vinet/internal/db"
	"github.com/alalfymansour/vinet/internal/export"
)

var exportFormat string
var resolveDestinations bool
var exportOutput string

var exportCmd = &cobra.Command{
	Use:   "export [file]",
	Short: "Export recorded traffic (JSON by default)",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		filter, ok := db.TimeFilters[timeFlag]
		if !ok {
			return fmt.Errorf("invalid --time value %q (want: today, 24h, week, 7d, month, 30d)", timeFlag)
		}
		database, err := openDatabase()
		if err != nil {
			return err
		}
		defer database.Close()
		records, err := export.Collect(database, filter, resolveDestinations)
		if err != nil {
			return err
		}

		format := exportFormat
		if len(args) == 1 {
			exportOutput = args[0]
		}
		if exportOutput != "" && !cmd.Flags().Changed("format") && strings.EqualFold(filepath.Ext(exportOutput), ".csv") {
			format = "csv"
		}
		var output io.Writer = os.Stdout
		if exportOutput != "" && exportOutput != "-" {
			outputFile, err := os.Create(exportOutput)
			if err != nil {
				return fmt.Errorf("creating export file: %w", err)
			}
			defer outputFile.Close()
			output = outputFile
		}
		return export.Write(output, records, format)
	},
}

var importCmd = &cobra.Command{
	Use:   "import <file>",
	Short: "Import a JSON or CSV export",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		input, err := openImportFile(args[0])
		if err != nil {
			return err
		}
		defer input.Close()
		data, err := io.ReadAll(input)
		if err != nil {
			return fmt.Errorf("reading import: %w", err)
		}
		records, err := export.Parse(data)
		if err != nil {
			return err
		}
		if len(records) == 0 {
			return fmt.Errorf("import contains no traffic records")
		}

		database, err := openDatabase()
		if err != nil {
			return err
		}
		defer database.Close()
		count, err := export.Insert(database, records)
		if err != nil {
			return err
		}
		fmt.Printf("Imported %d traffic records.\n", count)
		return nil
	},
}

func openImportFile(path string) (*os.File, error) {
	if path == "-" {
		return nil, fmt.Errorf("stdin imports are not supported yet; provide a file path")
	}
	input, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("opening import file: %w", err)
	}
	return input, nil
}

func init() {
	exportCmd.Flags().StringVarP(&timeFlag, "time", "t", "today", "Time range: today, 24h, week, 7d, month, 30d")
	exportCmd.Flags().BoolVarP(&dayFlag, "day", "d", false, "Today")
	exportCmd.Flags().BoolVarP(&monthFlag, "month", "m", false, "This month")
	exportCmd.Flags().BoolVarP(&weekFlag, "week", "w", false, "This week")
	exportCmd.Flags().StringVarP(&exportFormat, "format", "f", "json", "Output format: json or csv")
	exportCmd.Flags().BoolVar(&resolveDestinations, "resolve", false, "Resolve destination IPs to hostnames (may be slow)")
	exportCmd.Flags().StringVarP(&exportOutput, "output", "o", "", "Output file (default: stdout)")
	rootCmd.AddCommand(importCmd)
}
