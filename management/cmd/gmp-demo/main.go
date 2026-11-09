package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"

	"github.com/snack8310/gmp/management/internal/application"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	report, err := application.RunDemoFromEnv(context.Background(), os.LookupEnv, logger)
	if err != nil {
		logger.Error("mysql_demo_failed", "error_type", fmt.Sprintf("%T", err))
		os.Exit(1)
	}
	fmt.Printf("MySQL demo completed: %d synthetic executions recorded with receipts.\n", len(report.Executions))
}
