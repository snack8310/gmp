package management_test

import (
	"strings"
	"testing"

	"github.com/snack8310/gmp/management/internal/management"
)

func TestMySQLConfigRequiresExplicitConnectionAndPoolSettings(t *testing.T) {
	_, err := management.MySQLConfigFromEnv(func(string) (string, bool) { return "", false })
	if err == nil || !strings.Contains(err.Error(), "MYSQL_DSN") {
		t.Fatalf("got %v, want an error naming the missing MYSQL_DSN setting", err)
	}

	_, err = management.MySQLConfigFromEnv(mapEnv(map[string]string{
		"MYSQL_DSN": "tcp(127.0.0.1:3306)/gmp",
	}))
	if err == nil || !strings.Contains(err.Error(), "MYSQL_MAX_OPEN_CONNS") {
		t.Fatalf("got %v, want an error naming the missing MYSQL_MAX_OPEN_CONNS setting", err)
	}
}

func TestMySQLConfigRejectsInvalidPoolBounds(t *testing.T) {
	_, err := management.MySQLConfigFromEnv(mapEnv(map[string]string{
		"MYSQL_DSN":                   "tcp(127.0.0.1:3306)/gmp",
		"MYSQL_MAX_OPEN_CONNS":        "4",
		"MYSQL_MAX_IDLE_CONNS":        "5",
		"MYSQL_CONN_MAX_LIFETIME_SEC": "60",
	}))
	if err == nil || !strings.Contains(err.Error(), "MYSQL_MAX_IDLE_CONNS") {
		t.Fatalf("got %v, want invalid idle/open bounds to be rejected", err)
	}
}

func mapEnv(values map[string]string) func(string) (string, bool) {
	return func(key string) (string, bool) {
		value, ok := values[key]
		return value, ok
	}
}
