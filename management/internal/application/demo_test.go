package application_test

import (
	"context"
	"strings"
	"testing"

	"github.com/snack8310/gmp/management/internal/application"
)

func TestDemoRequiresExplicitMySQLConfig(t *testing.T) {
	_, err := application.RunDemoFromEnv(context.Background(), func(string) (string, bool) {
		return "", false
	}, runtimeTestLogger())
	if err == nil {
		t.Fatal("demo started without required MySQL configuration")
	}
	if !strings.Contains(err.Error(), "MYSQL_DSN") {
		t.Fatalf("missing configuration error = %v, want the required MYSQL_DSN field", err)
	}
}
