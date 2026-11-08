package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/snack8310/gmp/management/internal/httpapi"
	"github.com/snack8310/gmp/management/internal/management"
)

func main() {
	addr := flag.String("addr", "", "listen address, for example 127.0.0.1:8081")
	flag.Parse()
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	if *addr == "" {
		logger.Error("missing_listen_address")
		os.Exit(2)
	}
	config, err := management.MySQLConfigFromEnv(os.LookupEnv)
	if err != nil {
		logger.Error("mysql_configuration_invalid", "error", err.Error())
		os.Exit(2)
	}
	store, err := management.OpenMySQLStore(context.Background(), config, logger)
	if err != nil {
		logger.Error("mysql_store_start_failed", "error_type", fmt.Sprintf("%T", err))
		os.Exit(1)
	}
	defer func() {
		if err := store.Close(); err != nil {
			logger.Error("mysql_store_close_failed", "error_type", fmt.Sprintf("%T", err))
		}
	}()
	server := &http.Server{
		Addr:              *addr,
		Handler:           httpapi.NewHandler(store, logger),
		ReadHeaderTimeout: 5 * time.Second,
	}
	logger.Info("campaign_api_starting", "addr", *addr)
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		logger.Error("campaign_api_stopped", "error", err.Error())
		os.Exit(1)
	}
}
