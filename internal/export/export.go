// Package export reads and writes ViNet traffic records as JSON or CSV.
package export

import (
	"bytes"
	"database/sql"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"strconv"
	"strings"
)

// Record is a single traffic entry as stored in the traffic table.
type Record struct {
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

// Collect queries every traffic record newer than the given SQLite datetime
// modifier. When resolve is true, destination IPs are resolved to hostnames,
// which may be slow for large result sets.
func Collect(database *sql.DB, filter string, resolve bool) ([]Record, error) {
	rows, err := database.Query(`SELECT COALESCE(timestamp,''), COALESCE(pid,0), COALESCE(process_name,''), COALESCE(dest_ip,''), COALESCE(executable_path,''), COALESCE(family,2), COALESCE(protocol,0), COALESCE(dest_port,0), COALESCE(ifindex,0), COALESCE(bytes_sent,0), COALESCE(bytes_recv,0) FROM traffic WHERE timestamp >= datetime('now', ?) ORDER BY timestamp`, filter)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	records := make([]Record, 0)
	for rows.Next() {
		var r Record
		if err := rows.Scan(&r.Timestamp, &r.PID, &r.Process, &r.Destination, &r.Executable, &r.Family, &r.Protocol, &r.DestinationPort, &r.InterfaceIndex, &r.BytesSent, &r.BytesReceived); err != nil {
			return nil, err
		}
		if resolve && r.Destination != "" {
			names, _ := net.LookupAddr(r.Destination)
			if len(names) > 0 {
				r.Hostname = names[0]
			}
		}
		records = append(records, r)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return records, nil
}

// Write encodes records as JSON or CSV.
func Write(output io.Writer, records []Record, format string) error {
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
}

// Parse decodes JSON or CSV import data. The format is detected from the
// content: JSON when the payload starts with '[' or '{', CSV otherwise.
func Parse(data []byte) ([]Record, error) {
	format := "csv"
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) > 0 && (trimmed[0] == '[' || trimmed[0] == '{') {
		format = "json"
	}
	switch format {
	case "json":
		var records []Record
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
		var records []Record
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
			records = append(records, Record{Timestamp: field(row, "timestamp"), PID: uint32(pid), Process: field(row, "process"), Destination: field(row, "destination"), Hostname: field(row, "hostname"), Executable: field(row, "executable"), Family: uint16(family), Protocol: uint8(protocol), DestinationPort: uint16(port), InterfaceIndex: uint32(ifindex), BytesSent: sent, BytesReceived: received})
		}
		return records, nil
	}
	return nil, fmt.Errorf("unsupported import format")
}

// Insert writes records into the traffic table in a single transaction. It
// fails without importing anything when a record is missing timestamp or
// process. It returns the number of inserted records.
func Insert(database *sql.DB, records []Record) (int, error) {
	tx, err := database.Begin()
	if err != nil {
		return 0, err
	}
	stmt, err := tx.Prepare("INSERT INTO traffic (timestamp, pid, process_name, executable_path, dest_ip, family, protocol, dest_port, ifindex, bytes_sent, bytes_recv) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)")
	if err != nil {
		_ = tx.Rollback()
		return 0, err
	}
	defer stmt.Close()
	for _, record := range records {
		if record.Timestamp == "" || record.Process == "" {
			_ = tx.Rollback()
			return 0, fmt.Errorf("import contains a record with missing timestamp or process")
		}
		if _, err := stmt.Exec(record.Timestamp, record.PID, record.Process, record.Executable, record.Destination, record.Family, record.Protocol, record.DestinationPort, record.InterfaceIndex, record.BytesSent, record.BytesReceived); err != nil {
			_ = tx.Rollback()
			return 0, fmt.Errorf("importing traffic: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return len(records), nil
}
