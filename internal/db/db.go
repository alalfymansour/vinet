package db

import (
	"context"
	"database/sql"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

func InitDB() (*sql.DB, error) {
	if path := os.Getenv("VINET_DB"); path != "" {
		return InitDBAt(path)
	}
	return InitDBAt(DefaultPath())
}

// DefaultPath returns the database selected for the current installation.
func DefaultPath() string {
	if path := os.Getenv("VINET_DB"); path != "" {
		return path
	}
	configDir, err := os.UserConfigDir()
	if err != nil {
		return filepath.Join(".", "vinet", "data.db")
	}
	userDB := filepath.Join(configDir, "vinet", "data.db")

	// Installed services use a shared database. Prefer it when it exists so
	// the unprivileged CLI and root collector see the same history.
	systemDB := "/var/lib/vinet/data.db"
	if _, err := os.Stat(systemDB); err == nil {
		return systemDB
	}
	return userDB
}

// InitDBAt opens and migrates a database at an explicit path. Keeping the
// path outside this package makes service deployments deterministic.
func InitDBAt(dbPath string) (*sql.DB, error) {
	if err := os.MkdirAll(filepath.Dir(dbPath), 0700); err != nil {
		return nil, err
	}
	database, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, err
	}
	fileMode := os.FileMode(0600)
	if filepath.Clean(dbPath) == "/var/lib/vinet/data.db" {
		fileMode = 0660
	}
	_ = os.Chmod(dbPath, fileMode)
	database.SetMaxOpenConns(1)
	if _, err := database.Exec("PRAGMA busy_timeout=5000; PRAGMA journal_mode=WAL; PRAGMA foreign_keys=ON;"); err != nil {
		database.Close()
		return nil, fmt.Errorf("configuring sqlite: %w", err)
	}

	schema := `
	CREATE TABLE IF NOT EXISTS traffic (
        id INTEGER PRIMARY KEY AUTOINCREMENT,
        timestamp DATETIME DEFAULT CURRENT_TIMESTAMP,
        pid INTEGER,
		process_name TEXT,
		executable_path TEXT,
		dest_ip TEXT,
		family INTEGER DEFAULT 2,
		protocol INTEGER DEFAULT 0,
		dest_port INTEGER DEFAULT 0,
		ifindex INTEGER DEFAULT 0,
        bytes_sent INTEGER,
        bytes_recv INTEGER
	);
	CREATE TABLE IF NOT EXISTS collector_state (
		id INTEGER PRIMARY KEY CHECK (id = 1),
		last_seen DATETIME NOT NULL,
		last_error TEXT NOT NULL DEFAULT ''
	);
	`

	if _, err := database.Exec(schema); err != nil {
		database.Close()
		return nil, fmt.Errorf("failed to create schema: %w", err)
	}

	for _, column := range []struct{ name, definition string }{
		{"executable_path", "TEXT"}, {"dest_ip", "TEXT"}, {"family", "INTEGER DEFAULT 2"},
		{"protocol", "INTEGER DEFAULT 0"}, {"dest_port", "INTEGER DEFAULT 0"}, {"ifindex", "INTEGER DEFAULT 0"},
	} {
		var exists int
		if err := database.QueryRow("SELECT COUNT(*) FROM pragma_table_info('traffic') WHERE name=?", column.name).Scan(&exists); err != nil {
			database.Close()
			return nil, fmt.Errorf("checking schema column %s: %w", column.name, err)
		}
		if exists == 0 {
			if _, err := database.Exec("ALTER TABLE traffic ADD COLUMN " + column.name + " " + column.definition); err != nil {
				database.Close()
				return nil, fmt.Errorf("migrating %s: %w", column.name, err)
			}
		}
	}
	if _, err := database.Exec("CREATE INDEX IF NOT EXISTS idx_dest_ip ON traffic(dest_ip)"); err != nil {
		database.Close()
		return nil, fmt.Errorf("creating destination index: %w", err)
	}
	if _, err := database.Exec(`
		CREATE INDEX IF NOT EXISTS idx_process ON traffic(process_name);
		CREATE INDEX IF NOT EXISTS idx_timestamp ON traffic(timestamp);
		CREATE INDEX IF NOT EXISTS idx_process_timestamp ON traffic(process_name, timestamp);
		CREATE INDEX IF NOT EXISTS idx_destination_timestamp ON traffic(dest_ip, timestamp);
	`); err != nil {
		database.Close()
		return nil, fmt.Errorf("creating indexes: %w", err)
	}

	return database, nil
}

func FormatBytes(b int64) string {
	const unit = 1024
	if b < 0 {
		return "-" + FormatBytes(-b)
	}
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	units := "KMGTPE"
	if exp >= len(units) {
		return fmt.Sprintf("%.2f EiB", float64(b)/float64(div))
	}
	return fmt.Sprintf("%.2f %ciB", float64(b)/float64(div), units[exp])
}

func FormatIP(ip uint32) string {
	return fmt.Sprintf("%d.%d.%d.%d", byte(ip>>24), byte(ip>>16), byte(ip>>8), byte(ip))
}

func ProtocolName(protocol int) string {
	switch protocol {
	case 6:
		return "tcp"
	case 17:
		return "udp"
	default:
		return fmt.Sprintf("ip/%d", protocol)
	}
}

// ParseBytes parses values such as 500KiB, 2MiB, 1GB, or 1000B.
func ParseBytes(value string) (int64, error) {
	value = strings.TrimSuffix(strings.TrimSpace(value), "/s")
	match := regexp.MustCompile(`^\s*([0-9]+(?:\.[0-9]+)?)\s*([KMGTPE]?)(i?B)?\s*$`).FindStringSubmatch(value)
	if match == nil {
		return 0, fmt.Errorf("invalid byte size %q", value)
	}
	number, err := strconv.ParseFloat(match[1], 64)
	if err != nil {
		return 0, err
	}
	units := "BKMGTPE"
	exponent := strings.Index(units, match[2])
	if exponent < 0 {
		return 0, fmt.Errorf("invalid byte unit %q", match[2])
	}
	base := 1000.0
	if strings.HasPrefix(match[3], "i") || match[2] == "" {
		base = 1024.0
	}
	result := number * math.Pow(base, float64(exponent))
	if result > math.MaxInt64 {
		return 0, fmt.Errorf("byte size %q is too large", value)
	}
	return int64(result), nil
}

func StartPruner(ctx context.Context, database *sql.DB) {
	ticker := time.NewTicker(1 * time.Hour)
	defer ticker.Stop()

	prune := func() {
		retention := 30
		if value := os.Getenv("VINET_RETENTION_DAYS"); value != "" {
			if parsed, err := strconv.Atoi(value); err == nil && parsed > 0 {
				retention = parsed
			}
		}
		_, err := database.Exec("DELETE FROM traffic WHERE timestamp < datetime('now', ?)", fmt.Sprintf("-%d days", retention))
		if err != nil {
			fmt.Printf("prune traffic: %v\n", err)
		}
	}

	prune()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			prune()
		}
	}
}
