package ui

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/table"

	"github.com/alalfymansour/vinet/internal/db"
)

func (m *model) updateSummary() {
	m.err = ""
	m.health = "collector: unknown"
	var lastSeen, lastError string
	if err := m.db.QueryRow("SELECT COALESCE(last_seen, ''), COALESCE(last_error, '') FROM collector_state WHERE id=1").Scan(&lastSeen, &lastError); err == nil && lastSeen != "" {
		status := "active"
		if seenAt, parseErr := time.Parse("2006-01-02 15:04:05", lastSeen); parseErr == nil && time.Since(seenAt.UTC()) > 15*time.Second {
			status = "stale"
		}
		m.health = "collector: " + status
		if lastError != "" {
			m.health += " (" + lastError + ")"
		}
	}
	interfaceFilter := ""
	args := []any{m.timeRange}
	if m.interfaceIndex > 0 {
		interfaceFilter = " AND ifindex = ?"
		args = append(args, m.interfaceIndex)
	}
	processFilter := ""
	if m.filter != "" {
		processFilter = " AND process_name LIKE ?"
		args = append(args, "%"+m.filter+"%")
	}
	// Global totals for the header (across ALL processes, not just top 12).
	if err := m.db.QueryRow(`
		SELECT COALESCE(SUM(bytes_recv), 0), COALESCE(SUM(bytes_sent), 0)
		FROM traffic
		WHERE timestamp >= datetime('now', ?)
	`+interfaceFilter+processFilter, args...).Scan(&m.totalDown, &m.totalUp); err != nil {
		m.err = err.Error()
		return
	}

	rows, err := m.db.Query(`
		SELECT process_name, SUM(bytes_recv), SUM(bytes_sent)
		FROM traffic
		WHERE timestamp >= datetime('now', ?)
	`+interfaceFilter+processFilter+`
		GROUP BY process_name
		ORDER BY SUM(bytes_recv) DESC
	`, args...)
	if err != nil {
		m.err = err.Error()
		return
	}

	type procData struct {
		name     string
		down, up int64
	}
	var data []procData
	for rows.Next() {
		var d procData
		if err := rows.Scan(&d.name, &d.down, &d.up); err != nil {
			m.err = err.Error()
			rows.Close()
			return
		}
		data = append(data, d)
	}
	rows.Close()

	// Live rates (bytes/sec) over the last 30 seconds. The collector normally
	// polls every five seconds, so this remains responsive while tolerating a
	// delayed tick during system load or service startup.
	rateArgs := []any{}
	rateFilter := ""
	if m.interfaceIndex > 0 {
		rateFilter = " AND ifindex = ?"
		rateArgs = append(rateArgs, m.interfaceIndex)
	}
	rateRows, err := m.db.Query(`
		SELECT process_name, SUM(bytes_recv), MAX((julianday('now') - julianday(MIN(timestamp))) * 86400.0, 1)
		FROM traffic
		WHERE timestamp >= datetime('now', '-30 seconds')
	`+rateFilter+`
		GROUP BY process_name;
	`, rateArgs...)
	if err != nil {
		m.err = err.Error()
		return
	}
	liveRates := make(map[string]int64)
	for rateRows.Next() {
		var name string
		var down int64
		var seconds float64
		if err := rateRows.Scan(&name, &down, &seconds); err != nil {
			m.err = err.Error()
			rateRows.Close()
			return
		}
		liveRates[name] = int64(float64(down) / seconds)
	}
	rateRows.Close()

	// Apply the active sort mode.
	switch m.sortMode {
	case 1: // Upload
		sort.Slice(data, func(i, j int) bool { return data[i].up > data[j].up })
	case 2: // Rate
		sort.Slice(data, func(i, j int) bool { return liveRates[data[i].name] > liveRates[data[j].name] })
	default: // Download
		sort.Slice(data, func(i, j int) bool { return data[i].down > data[j].down })
	}

	var newRows []table.Row
	for _, d := range data {
		newRows = append(newRows, table.Row{
			d.name,
			db.FormatBytes(d.down),
			db.FormatBytes(d.up),
			db.FormatBytes(liveRates[d.name]) + "/s",
		})
	}
	m.table.SetRows(newRows)
}

func (m *model) updateDetail() {
	m.err = ""
	interfaceFilter := ""
	args := []any{m.selected, m.timeRange}
	if m.interfaceIndex > 0 {
		interfaceFilter = " AND ifindex = ?"
		args = append(args, m.interfaceIndex)
	}
	rows, err := m.db.Query(`
		SELECT COALESCE(dest_ip, ''), dest_port, protocol, ifindex, SUM(bytes_recv), SUM(bytes_sent)
		FROM traffic
		WHERE process_name = ? AND timestamp >= datetime('now', ?)
	`+interfaceFilter+`
		GROUP BY dest_ip, dest_port, protocol, ifindex
		ORDER BY SUM(bytes_recv) + SUM(bytes_sent) DESC
	`, args...)
	if err != nil {
		m.err = err.Error()
		return
	}

	type ipData struct {
		ip       string
		port     uint16
		protocol uint8
		ifindex  uint32
		down, up int64
	}
	var data []ipData
	for rows.Next() {
		var d ipData
		if err := rows.Scan(&d.ip, &d.port, &d.protocol, &d.ifindex, &d.down, &d.up); err != nil {
			m.err = err.Error()
			rows.Close()
			return
		}
		if d.ip == "0.0.0.0" {
			d.ip = "localhost/listen"
		}
		if d.port != 0 {
			if strings.Contains(d.ip, ":") {
				d.ip = fmt.Sprintf("[%s]:%d/%s", d.ip, d.port, db.ProtocolName(int(d.protocol)))
			} else {
				d.ip = fmt.Sprintf("%s:%d/%s", d.ip, d.port, db.ProtocolName(int(d.protocol)))
			}
		}
		data = append(data, d)
	}
	rows.Close()

	var newRows []table.Row
	for _, d := range data {
		newRows = append(newRows, table.Row{
			d.ip,
			db.FormatBytes(d.down),
			db.FormatBytes(d.up),
		})
	}
	m.table.SetRows(newRows)
}
