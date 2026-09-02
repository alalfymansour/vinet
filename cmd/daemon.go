package cmd

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/alalfymansour/vinet/internal/metrics"
	"github.com/alalfymansour/vinet/internal/tracker"
	"github.com/spf13/cobra"
)

var daemonCmd = &cobra.Command{
	Use:   "daemon",
	Short: "Starts the background tracking service",
	RunE:  runDaemon,
}

var metricsAddress string

func runDaemon(cmd *cobra.Command, args []string) error {
	database, err := openDatabase()
	if err != nil {
		return err
	}
	defer database.Close()

	log.Println("ViNet daemon started successfully. Database initialized.")
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if metricsAddress != "" {
		if err := metrics.Start(ctx, database, metricsAddress); err != nil {
			return err
		}
		log.Printf("Prometheus metrics listening on %s", metricsAddress)
	}
	if err := tracker.StartEbpf(ctx, database); err != nil && err != context.Canceled {
		return err
	}
	log.Println("ViNet daemon stopped.")
	return nil
}

func init() {
	daemonCmd.Flags().StringVar(&metricsAddress, "metrics-addr", "", "Prometheus metrics address, e.g. 127.0.0.1:9109")
}
