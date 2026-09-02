package cmd

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
)

var exportFormat string
var resolveDestinations bool
var exportOutput string

type exportRecord struct {
	Timestamp       string `json:"timestamp"`
	PID             uint32 `json:"pid"`
	Process         string `json:"process"`
	Destination     string `json:"destination"`
	Hostname        string `json:"hostname,omitempty"`
	Executable      string `json:"executable"`
	Family          uint16 `json:"family"`
	Protocol        uint8  `json:"protocol"`
	DestinationPort uint16 `json:"destination_port"`
	InterfaceIndex  uint32 `json:"interface_index"`
	BytesSent       int64  `json:"bytes_sent"`
	BytesReceived   int64  `json:"bytes_received"`
}

var exportCmd = &cobra.Command{
	Use:   "export [file]",
	Short: "Export recorded traffic (JSON by default)",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		filter, ok := timeFilters[timeFlag]
		if !ok {
			return fmt.Errorf("invalid --time value %q (want: today, 24h, week, 7d, month, 30d)", timeFlag)
		}
		database, err := openDatabase()
		if err != nil {
			return err
		}
		defer database.Close()
		queryArgs := []any{filter}
		rows, err := database.Query(`SELECT COALESCE(timestamp,''), COALESCE(pid,0), COALESCE(process_name,''), COALESCE(dest_ip,''), COALESCE(executable_path,''), COALESCE(family,2), COALESCE(protocol,0), COALESCE(dest_port,0), COALESCE(ifindex,0), COALESCE(bytes_sent,0), COALESCE(bytes_recv,0) FROM traffic WHERE timestamp >= datetime('now', ?) ORDER BY timestamp`, queryArgs...)
		if err != nil {
			return err
		}
		defer rows.Close()
		records := make([]exportRecord, 0)
		for rows.Next() {
			var r exportRecord
			if err := rows.Scan(&r.Timestamp, &r.PID, &r.Process, &r.Destination, &r.Executable, &r.Family, &r.Protocol, &r.DestinationPort, &r.InterfaceIndex, &r.BytesSent, &r.BytesReceived); err != nil {
				return err
			}
			if resolveDestinations && r.Destination != "" {
				names, _ := net.LookupAddr(r.Destination)
				if len(names) > 0 {
					r.Hostname = names[0]
				}
			}
			records = append(records, r)
		}
		if err := rows.Err(); err != nil {
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
		var outputFile *os.File
		if exportOutput != "" && exportOutput != "-" {
			outputFile, err = os.Create(exportOutput)
			if err != nil {
				return fmt.Errorf("creating export file: %w", err)
			}
			defer outputFile.Close()
			output = outputFile
		}
		switch format {
		case "json":
			return json.NewEncoder(output).Encode(records)
		case "csv":
			w := csv.NewWriter(output)
			if err := w.Write([]string{"timestamp", "pid", "process", "destination", "hostname", "executable", "family", "protocol", "destination_port", "interface_index", "bytes_sent", "bytes_received"}); err != nil {
				return err
			}
			for _, r := range records {
				if err := w.Write([]string{r.Timestamp, fmt.Sprint(r.PID), r.Process, r.Destination, r.Hostname, r.Executable, fmt.Sprint(r.Family), fmt.Sprint(r.Protocol), fmt.Sprint(r.DestinationPort), fmt.Sprint(r.InterfaceIndex), fmt.Sprint(r.BytesSent), fmt.Sprint(r.BytesReceived)}); err != nil {
					return err
				}
			}
			w.Flush()
			return w.Error()
		default:
			return fmt.Errorf("invalid --format %q (want json or csv)", format)
		}
	},
}

func init() {
	exportCmd.Flags().StringVarP(&timeFlag, "time", "t", "today", "Time range: today, 24h, week, 7d, month, 30d")
	exportCmd.Flags().BoolVarP(&dayFlag, "day", "d", false, "Today")
	exportCmd.Flags().BoolVarP(&monthFlag, "month", "m", false, "This month")
	exportCmd.Flags().BoolVarP(&weekFlag, "week", "w", false, "This week")
	exportCmd.Flags().StringVarP(&exportFormat, "format", "f", "json", "Output format: json or csv")
	exportCmd.Flags().BoolVar(&resolveDestinations, "resolve", false, "Resolve destination IPs to hostnames (may be slow)")
	exportCmd.Flags().StringVarP(&exportOutput, "output", "o", "", "Output file (default: stdout)")
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
		records, err := parseImport(data)
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
		tx, err := database.Begin()
		if err != nil {
			return err
		}
		stmt, err := tx.Prepare("INSERT INTO traffic (timestamp, pid, process_name, executable_path, dest_ip, family, protocol, dest_port, ifindex, bytes_sent, bytes_recv) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)")
		if err != nil {
			_ = tx.Rollback()
			return err
		}
		defer stmt.Close()
		for _, record := range records {
			if record.Timestamp == "" || record.Process == "" {
				_ = tx.Rollback()
				return fmt.Errorf("import contains a record with missing timestamp or process")
			}
			if _, err := stmt.Exec(record.Timestamp, record.PID, record.Process, record.Executable, record.Destination, record.Family, record.Protocol, record.DestinationPort, record.InterfaceIndex, record.BytesSent, record.BytesReceived); err != nil {
				_ = tx.Rollback()
				return fmt.Errorf("importing traffic: %w", err)
			}
		}
		if err := tx.Commit(); err != nil {
			return err
		}
		fmt.Printf("Imported %d traffic records.\n", len(records))
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

func parseImport(data []byte) ([]exportRecord, error) {
	format := "csv"
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) > 0 && (trimmed[0] == '[' || trimmed[0] == '{') {
		format = "json"
	}
	switch format {
	case "json":
		var records []exportRecord
		if err := json.Unmarshal(data, &records); err != nil {
			return nil, fmt.Errorf("parsing JSON import: %w", err)
		}
		return records, nil
	case "csv":
		reader := csv.NewReader(bytes.NewReader(data))
		rows, err := reader.ReadAll()
		if err != nil {
			return nil, fmt.Errorf("parsing CSV import: %w", err)
		}
		if len(rows) < 2 {
			return nil, fmt.Errorf("CSV import must contain a header and at least one record")
		}
		columns := make(map[string]int, len(rows[0]))
		for index, name := range rows[0] {
			columns[strings.TrimSpace(name)] = index
		}
		field := func(row []string, name string) string {
			index, ok := columns[name]
			if !ok || index >= len(row) {
				return ""
			}
			return row[index]
		}
		var records []exportRecord
		for rowIndex, row := range rows[1:] {
			parseUint := func(name string, bits int) (uint64, error) {
				value := field(row, name)
				if value == "" {
					return 0, nil
				}
				parsed, err := strconv.ParseUint(value, 10, bits)
				if err != nil {
					return 0, fmt.Errorf("row %d: invalid %s", rowIndex+2, name)
				}
				return parsed, nil
			}
			parseInt := func(name string) (int64, error) {
				value := field(row, name)
				if value == "" {
					return 0, nil
				}
				parsed, err := strconv.ParseInt(value, 10, 64)
				if err != nil {
					return 0, fmt.Errorf("row %d: invalid %s", rowIndex+2, name)
				}
				return parsed, nil
			}
			pid, err := parseUint("pid", 32)
			if err != nil {
				return nil, err
			}
			family, err := parseUint("family", 16)
			if err != nil {
				return nil, err
			}
			protocol, err := parseUint("protocol", 8)
			if err != nil {
				return nil, err
			}
			port, err := parseUint("destination_port", 16)
			if err != nil {
				return nil, err
			}
			ifindex, err := parseUint("interface_index", 32)
			if err != nil {
				return nil, err
			}
			sent, err := parseInt("bytes_sent")
			if err != nil {
				return nil, err
			}
			received, err := parseInt("bytes_received")
			if err != nil {
				return nil, err
			}
			records = append(records, exportRecord{Timestamp: field(row, "timestamp"), PID: uint32(pid), Process: field(row, "process"), Destination: field(row, "destination"), Hostname: field(row, "hostname"), Executable: field(row, "executable"), Family: uint16(family), Protocol: uint8(protocol), DestinationPort: uint16(port), InterfaceIndex: uint32(ifindex), BytesSent: sent, BytesReceived: received})
		}
		return records, nil
	}
	return nil, fmt.Errorf("unsupported import format")
}

func init() {
	rootCmd.AddCommand(importCmd)
}
