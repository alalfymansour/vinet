package db

import (
	"database/sql"
	"testing"

	_ "modernc.org/sqlite"
)

func TestFormatBytes(t *testing.T) {
	cases := map[int64]string{0: "0 B", 1023: "1023 B", 1024: "1.00 KiB", 1024 * 1024: "1.00 MiB"}
	for input, want := range cases {
		if got := FormatBytes(input); got != want {
			t.Errorf("FormatBytes(%d) = %q, want %q", input, got, want)
		}
	}
}

func TestParseBytes(t *testing.T) {
	for input, want := range map[string]int64{"1KiB": 1024, "2MiB/s": 2 * 1024 * 1024, "1KB": 1000, "500B": 500} {
		got, err := ParseBytes(input)
		if err != nil || got != want {
			t.Errorf("ParseBytes(%q) = %d, %v; want %d", input, got, err, want)
		}
	}
	if _, err := ParseBytes("not-bytes"); err == nil {
		t.Error("expected invalid byte size error")
	}
}

func TestInitDBAtMigratesAndPersists(t *testing.T) {
	database, err := InitDBAt(t.TempDir() + "/traffic.db")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := database.Exec("INSERT INTO traffic (process_name, dest_ip, bytes_sent, bytes_recv) VALUES ('test', '127.0.0.1', 2, 3)"); err != nil {
		t.Fatal(err)
	}
	var total int
	if err := database.QueryRow("SELECT COUNT(*) FROM traffic").Scan(&total); err != nil {
		t.Fatal(err)
	}
	if total != 1 {
		t.Fatalf("got %d rows, want 1", total)
	}
	database.Close()
}

func TestInitDBAtMigratesLegacySchema(t *testing.T) {
	path := t.TempDir() + "/legacy.db"
	legacy, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := legacy.Exec("CREATE TABLE traffic (id INTEGER PRIMARY KEY, timestamp DATETIME, pid INTEGER, process_name TEXT, bytes_sent INTEGER, bytes_recv INTEGER)"); err != nil {
		t.Fatal(err)
	}
	if err := legacy.Close(); err != nil {
		t.Fatal(err)
	}
	database, err := InitDBAt(path)
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	var columns int
	if err := database.QueryRow("SELECT COUNT(*) FROM pragma_table_info('traffic') WHERE name IN ('executable_path','family','protocol','dest_port','ifindex')").Scan(&columns); err != nil {
		t.Fatal(err)
	}
	if columns != 5 {
		t.Fatalf("got %d migrated columns, want 5", columns)
	}
}
