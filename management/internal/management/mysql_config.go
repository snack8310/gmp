package management

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/go-sql-driver/mysql"
)

type MySQLConfig struct {
	DSN             string
	MaxOpenConns    int
	MaxIdleConns    int
	ConnMaxLifetime time.Duration
}

func MySQLConfigFromEnv(lookup func(string) (string, bool)) (MySQLConfig, error) {
	if lookup == nil {
		return MySQLConfig{}, fmt.Errorf("management: environment lookup is required")
	}
	readRequired := func(name string) (string, error) {
		value, ok := lookup(name)
		if !ok || strings.TrimSpace(value) == "" {
			return "", fmt.Errorf("management: required environment variable %s is missing", name)
		}
		return strings.TrimSpace(value), nil
	}

	dsn, err := readRequired("MYSQL_DSN")
	if err != nil {
		return MySQLConfig{}, err
	}
	parsed, err := mysql.ParseDSN(dsn)
	if err != nil || parsed.DBName == "" {
		return MySQLConfig{}, fmt.Errorf("management: MYSQL_DSN must be a valid MySQL DSN with a database name")
	}

	maxOpen, err := requiredPositiveInt(readRequired, "MYSQL_MAX_OPEN_CONNS")
	if err != nil {
		return MySQLConfig{}, err
	}
	maxIdle, err := requiredNonNegativeInt(readRequired, "MYSQL_MAX_IDLE_CONNS")
	if err != nil {
		return MySQLConfig{}, err
	}
	lifetimeSeconds, err := requiredPositiveInt(readRequired, "MYSQL_CONN_MAX_LIFETIME_SEC")
	if err != nil {
		return MySQLConfig{}, err
	}
	if maxIdle > maxOpen {
		return MySQLConfig{}, fmt.Errorf("management: MYSQL_MAX_IDLE_CONNS cannot exceed MYSQL_MAX_OPEN_CONNS")
	}
	return MySQLConfig{
		DSN:             dsn,
		MaxOpenConns:    maxOpen,
		MaxIdleConns:    maxIdle,
		ConnMaxLifetime: time.Duration(lifetimeSeconds) * time.Second,
	}, nil
}

type requiredValue func(string) (string, error)

func requiredPositiveInt(read requiredValue, name string) (int, error) {
	value, err := read(name)
	if err != nil {
		return 0, err
	}
	parsed, err := strconv.Atoi(value)
	if err != nil || parsed <= 0 {
		return 0, fmt.Errorf("management: %s must be a positive integer", name)
	}
	return parsed, nil
}

func requiredNonNegativeInt(read requiredValue, name string) (int, error) {
	value, err := read(name)
	if err != nil {
		return 0, err
	}
	parsed, err := strconv.Atoi(value)
	if err != nil || parsed < 0 {
		return 0, fmt.Errorf("management: %s must be a non-negative integer", name)
	}
	return parsed, nil
}
