package metrics

import (
	"context"
	"database/sql"
	"fmt"
	"net"
	"net/http"
	"strconv"
)

// Start exposes low-cost collector metrics and returns after binding the
// socket. The server is shut down automatically with ctx.
func Start(ctx context.Context, database *sql.DB, address string) error {
	listener, err := net.Listen("tcp", address)
	if err != nil {
		return fmt.Errorf("listen for metrics: %w", err)
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/metrics", handler(database))
	server := &http.Server{Handler: mux}
	go func() { <-ctx.Done(); _ = server.Shutdown(context.Background()) }()
	go func() { _ = server.Serve(listener) }()
	return nil
}

func handler(database *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain; version=0.0.4")
		var down, up int64
		if err := database.QueryRow("SELECT COALESCE(SUM(bytes_recv),0), COALESCE(SUM(bytes_sent),0) FROM traffic WHERE timestamp >= datetime('now','-10 seconds')").Scan(&down, &up); err != nil {
			http.Error(w, "database unavailable", http.StatusServiceUnavailable)
			return
		}
		fmt.Fprintf(w, "vinet_bytes_received_total %d\nvinet_bytes_sent_total %d\n", down, up)
		rows, err := database.Query("SELECT process_name, SUM(bytes_recv), SUM(bytes_sent) FROM traffic WHERE timestamp >= datetime('now','-10 seconds') GROUP BY process_name")
		if err != nil {
			return
		}
		defer rows.Close()
		for rows.Next() {
			var name string
			var recv, sent int64
			if rows.Scan(&name, &recv, &sent) == nil {
				fmt.Fprintf(w, "vinet_process_bytes_received{process=%s} %d\n", strconv.Quote(name), recv)
				fmt.Fprintf(w, "vinet_process_bytes_sent{process=%s} %d\n", strconv.Quote(name), sent)
			}
		}
	}
}
