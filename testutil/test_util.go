// Package testutil provides shared test infrastructure for sqlex tests.
package testutil

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"sync"

	"github.com/joho/godotenv"
)

// ==================== Environment Variable Loading ====================

var envOnce sync.Once

// rootDir returns PROJECT_ROOT, or the parent of this file's directory.
// It returns "" if neither is available, so .env.test is resolved relative to the working directory.
func rootDir() string {
	if dir := os.Getenv("PROJECT_ROOT"); dir != "" {
		return dir
	}
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		return ""
	}
	return filepath.Dir(filepath.Dir(filename)) // Project root is the parent of testutil/
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
//
// A missing .env.test is allowed. Any other load error (e.g. a malformed file) panics:
// it is called from init() where no *testing.T exists, and ignoring it would silently
// skip every database-backed test while the run still passes.
func LoadTestEnv() {
	envOnce.Do(func() {
		envPath := filepath.Join(rootDir(), ".env.test")
		if err := godotenv.Load(envPath); err != nil && !errors.Is(err, fs.ErrNotExist) {
			panic(fmt.Sprintf("testutil: failed to load %s: %v", envPath, err))
		}
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
