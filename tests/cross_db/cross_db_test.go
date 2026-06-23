// cross_db_test.go — MySQL + PostgreSQL + SQL Server cross-DB integration test infrastructure
//
// This file defines shared structs, helper functions, and seed data for cross-DB tests.
// All tests run on MySQL, PostgreSQL, and SQL Server via the runWithSchema framework.
//
// Usage:
//
//	go test -v -run "TestCrossDB" -count=1 -timeout=180s -race ./tests/cross_db/
//
// MySQL only:
//
//	SQLX_POSTGRES_DSN=skip SQLX_SQLSERVER_DSN=skip go test -v -run "TestCrossDB" -count=1 ./tests/cross_db/
//
// PostgreSQL only:
//
//	SQLX_MYSQL_DSN=skip SQLX_SQLSERVER_DSN=skip go test -v -run "TestCrossDB" -count=1 ./tests/cross_db/
//
// SQL Server only:
//
//	SQLX_MYSQL_DSN=skip SQLX_POSTGRES_DSN=skip go test -v -run "TestCrossDB" -count=1 ./tests/cross_db/
package cross_db_test

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	sqlex "github.com/go-sqlex/sqlex"
)

// ---------- Cross-DB test helper functions ----------

// crossDBOnly skips environments without any remote database
func crossDBOnly(t *testing.T) {
	t.Helper()
	if !isTestPostgres && !isTestMysql && !isTestSqlserver {
		t.Skip("No remote database (MySQL/PostgreSQL/SQL Server) is available, skipping cross-DB test")
	}
}

// isMySQL checks if the current DB is MySQL
func isMySQL(db *sqlex.DB) bool {
	return db.DriverName() == "mysql"
}

// isPostgres checks if the current DB is PostgreSQL
func isPostgres(db *sqlex.DB) bool {
	return db.DriverName() == "postgres"
}

// isSQLServer checks if the current DB is SQL Server
func isSQLServer(db *sqlex.DB) bool {
	return db.DriverName() == "sqlserver"
}

// schemaForDB returns the corresponding DDL based on the current DB type
func schemaForDB(db *sqlex.DB, s Schema) (create, drop, now string) {
	switch {
	case isMySQL(db):
		return s.MySQL()
	case isSQLServer(db):
		return s.SQLServer()
	default:
		return s.Postgres()
	}
}

// selectTop1 converts "SELECT ... LIMIT 1" to SQL Server compatible "SELECT TOP 1 ..."
// Returns original SQL for MySQL/PostgreSQL
func selectTop1(db *sqlex.DB, query string) string {
	if !isSQLServer(db) {
		return query
	}
	// Remove trailing LIMIT 1
	q := strings.TrimSuffix(strings.TrimSpace(query), "LIMIT 1")
	q = strings.TrimSpace(q)
	// Replace SELECT → SELECT TOP 1
	q = strings.Replace(q, "SELECT ", "SELECT TOP 1 ", 1)
	return q
}

// dbLabel returns the database label for logging
func dbLabel(db *sqlex.DB) string {
	return strings.ToUpper(db.DriverName())
}

// ---------- Cross-DB compatible Schema ----------

// crossSchema uses SQLite DDL syntax, auto-converted via Schema.MySQL() / Schema.Postgres()
var crossSchema = Schema{
	Create: `
CREATE TABLE cross_users (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	name TEXT NOT NULL,
	email TEXT NOT NULL,
	age INTEGER DEFAULT 0,
	created_at TIMESTAMP DEFAULT now()
);

CREATE TABLE cross_orders (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	user_id INTEGER NOT NULL,
	amount REAL NOT NULL,
	status VARCHAR(50) DEFAULT 'pending'
);
`,
	Drop: `
DROP TABLE IF EXISTS cross_users;
DROP TABLE IF EXISTS cross_orders;
`,
}

// ---------- Common test structs ----------

// CrossUser is the user struct for cross-DB tests
type CrossUser struct {
	ID        int       `db:"id"`
	Name      string    `db:"name"`
	Email     string    `db:"email"`
	Age       int       `db:"age"`
	CreatedAt time.Time `db:"created_at"`
}

// CrossOrder is the order struct for cross-DB tests
type CrossOrder struct {
	ID     int     `db:"id"`
	UserID int     `db:"user_id"`
	Amount float64 `db:"amount"`
	Status string  `db:"status"`
}

// ---------- Seed data ----------

// seedCrossData inserts standard cross-DB test seed data
func seedCrossData(db *sqlex.DB, t *testing.T) {
	t.Helper()
	tx := db.MustBegin()
	tx.MustExec(`INSERT INTO cross_users (name, email, age) VALUES ('Alice', 'alice@example.com', 30)`)
	tx.MustExec(`INSERT INTO cross_users (name, email, age) VALUES ('Bob', 'bob@example.com', 25)`)
	tx.MustExec(`INSERT INTO cross_users (name, email, age) VALUES ('Charlie', 'charlie@example.com', 35)`)
	tx.MustExec(`INSERT INTO cross_orders (user_id, amount, status) VALUES (1, 99.9, 'paid')`)
	tx.MustExec(`INSERT INTO cross_orders (user_id, amount, status) VALUES (1, 50.0, 'pending')`)
	tx.MustExec(`INSERT INTO cross_orders (user_id, amount, status) VALUES (2, 200.0, 'paid')`)
	tx.Commit()
}

// ---------- Cross-DB test Hook implementations ----------

// crossTestHook is a Hook implementation for cross-DB tests that records call counts and queries
type crossTestHook struct {
	mu          sync.Mutex
	beforeCount int
	afterCount  int
	queries     []string
	errors      []error
	durations   []time.Duration
}

func (h *crossTestHook) BeforeQuery(ctx context.Context, event *sqlex.QueryEvent) context.Context {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.beforeCount++
	h.queries = append(h.queries, event.Query)
	return ctx
}

func (h *crossTestHook) AfterQuery(ctx context.Context, event *sqlex.QueryEvent) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.afterCount++
	h.errors = append(h.errors, event.Error)
	h.durations = append(h.durations, event.Duration)
}

// crossOrderHook records Hook execution order (used to verify onion model)
type crossOrderHook struct {
	name  string
	order *[]string
	mu    *sync.Mutex
}

func (h *crossOrderHook) BeforeQuery(ctx context.Context, event *sqlex.QueryEvent) context.Context {
	h.mu.Lock()
	defer h.mu.Unlock()
	*h.order = append(*h.order, "before:"+h.name)
	return ctx
}

func (h *crossOrderHook) AfterQuery(ctx context.Context, event *sqlex.QueryEvent) {
	h.mu.Lock()
	defer h.mu.Unlock()
	*h.order = append(*h.order, "after:"+h.name)
}

