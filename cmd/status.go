package cmd

import (
	"database/sql"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/alalfymansour/vinet/internal/db"
)

func runStatus() error {
	database, err := openDatabase()
	if err != nil {
		return fmt.Errorf("opening database: %w", err)
	}
	defer database.Close()

	var lastSeen, lastError sql.NullString
	err = database.QueryRow("SELECT last_seen, last_error FROM collector_state WHERE id=1").Scan(&lastSeen, &lastError)
	if err != nil && err != sql.ErrNoRows {
		return err
	}

	state := "not running"
	heartbeat := "never"
	collectorError := "none"
	var records int64
	if err := database.QueryRow("SELECT COUNT(*) FROM traffic").Scan(&records); err != nil {
		return err
	}
	if lastSeen.Valid && lastSeen.String != "" {
		heartbeat = lastSeen.String + " UTC"
		state = "running"
		if parsed, parseErr := time.Parse("2006-01-02 15:04:05", lastSeen.String); parseErr == nil && time.Since(parsed.UTC()) > 15*time.Second {
			state = "stale"
		}
	}
	if lastError.Valid && lastError.String != "" {
		collectorError = lastError.String
		state = "error"
	}

	fmt.Printf("ViNet status: %s\n", state)
	fmt.Printf("Service: %s\n", serviceStatus())
	fmt.Printf("Database: %s\n", dbPath())
	fmt.Printf("Records: %d\n", records)
	fmt.Printf("Last heartbeat: %s\n", heartbeat)
	fmt.Printf("Collector error: %s\n", collectorError)
	return nil
}

func serviceStatus() string {
	if _, err := exec.LookPath("systemctl"); err == nil {
		output, err := exec.Command("systemctl", "is-active", "vinet").Output()
		status := strings.TrimSpace(string(output))
		if status != "" {
			return "systemd: " + status
		}
		if err != nil {
			return "systemd: inactive"
		}
	}
	if _, err := exec.LookPath("rc-service"); err == nil {
		if err := exec.Command("rc-service", "vinet", "status").Run(); err == nil {
			return "openrc: started"
		}
		return "openrc: stopped"
	}
	return "not detected"
}

func dbPath() string {
	return db.DefaultPath()
}
