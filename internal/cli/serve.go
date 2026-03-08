package cli

import (
	"errors"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/spf13/cobra"

	"github.com/vector76/backlog_manager/internal/beadsserver"
	"github.com/vector76/backlog_manager/internal/config"
	"github.com/vector76/backlog_manager/internal/server"
)

const beadPollInterval = 60 * time.Second

func newServeCmd() *cobra.Command {
	var configPath string

	cmd := &cobra.Command{
		Use:   "serve",
		Short: "Start the HTTP server",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load(configPath)
			if err != nil {
				return fmt.Errorf("load config: %w", err)
			}

			st, err := server.NewStore(cfg.DataDir)
			if err != nil {
				return fmt.Errorf("open store: %w", err)
			}

			var monitor *server.BeadMonitor
			if cfg.BeadsServerURL != "" {
				client := beadsserver.New(cfg.BeadsServerURL)
				monitor = server.NewBeadMonitor(client, st, beadPollInterval)
				monitor.Start()
				log.Printf("bead monitor started, polling %s every %s", cfg.BeadsServerURL, beadPollInterval)
			}

			srv := server.New(cfg, st, monitor)
			log.Printf("starting server on %s", srv.Addr)
			if err := srv.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
				return err
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&configPath, "config", "config.json", "path to config file")
	return cmd
}
