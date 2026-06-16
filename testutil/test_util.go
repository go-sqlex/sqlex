package testutil

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sync"

	"github.com/joho/godotenv"
)

// ==================== Environment Variable Loading ====================

var envOnce sync.Once

func rootDir() string {
	rootDir := os.Getenv("PROJECT_ROOT")
	if rootDir == "" {
		// If PROJECT_ROOT env var is not set, try to auto-detect
		_, filename, _, _ := runtime.Caller(0)
		rootDir = filepath.Dir(filepath.Dir(filename)) // Assume project root is the parent of the current directory
	}
	return rootDir
}

// LoadTestEnv loads environment variables from .env.test in the project root (loaded only once).
//
// Environment variables:
//   - SQLX_MYSQL_DSN     — MySQL full DSN, e.g. "root:password@tcp(127.0.0.1:3306)/test?parseTime=true"
//   - SQLX_POSTGRES_DSN  — PostgreSQL full DSN, e.g. "postgres://user:password@127.0.0.1:5432/test?sslmode=disable"
//   - SQLX_SQLITE_DSN    — SQLite DSN, defaults to ":memory:"
//   - SQLX_SQLSERVER_DSN — SQL Server full DSN, e.g. "sqlserver://user:password@127.0.0.1:1433?database=test"
//   - SQLX_ORACLE_DSN    — Oracle full DSN, e.g. "oracle://user:password@host:1521/service_name"
//
// Set to "skip" to skip tests for that database.
func LoadTestEnv() {
	envOnce.Do(func() {
		envPath := filepath.Join(rootDir(), ".env.test")
		// Ignore file-not-found errors; allow running without .env.test
		_ = godotenv.Load(envPath)
	})
}

// EnvMust gets an environment variable; panics if empty
func EnvMust(s string) string {
	val := os.Getenv(s)
	if len(val) == 0 {
		panic(fmt.Errorf("empty env[%v], please check .env.test file", s))
	}
	return val
}

// EnvOr gets an environment variable; returns defaultVal if empty
func EnvOr(key, defaultVal string) string {
	val := os.Getenv(key)
	if val == "" {
		return defaultVal
	}
	return val
}
