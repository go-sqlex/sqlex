// pg_test.go — PostgreSQL-specific integration tests
//
// Coverage:
//  1. Database connectivity (Ping, connection close)
//  2. Standard CRUD, transactions, prepared statements, connection pool
//  3. New features: Hook system, JsonValue[T], Named queries, Context control
//  4. PostgreSQL-specific: JSON/JSONB, arrays, :: type casts, $N placeholders
//  5. Edge cases: nulls, type conversion errors, connection timeout, concurrency
//
// Run (PostgreSQL only):
//
//	go test -v -count=1 -timeout=120s ./tests/pg/
package pg_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	sqlex "github.com/go-sqlex/sqlex"
	"github.com/go-sqlex/sqlex/types"
)

// ---------- PostgreSQL test helpers ----------

// IntUser struct
type IntUser struct {
	ID        int       `db:"id"`
	Name      string    `db:"name"`
	Email     string    `db:"email"`
	Age       int       `db:"age"`
	CreatedAt time.Time `db:"created_at"`
}

// IntOrder struct
type IntOrder struct {
	ID     int     `db:"id"`
	UserID int     `db:"user_id"`
	Amount float64 `db:"amount"`
	Status string  `db:"status"`
}

// testHook is a test Hook implementation
type testHook struct {
	mu          sync.Mutex
	beforeCount int
	afterCount  int
	queries     []string
}

func (h *testHook) BeforeQuery(ctx context.Context, event *sqlex.QueryEvent) context.Context {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.beforeCount++
	h.queries = append(h.queries, event.Query)
	return ctx
}

func (h *testHook) AfterQuery(ctx context.Context, event *sqlex.QueryEvent) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.afterCount++
}

// orderHook validates Hook onion model call order
type orderHook struct {
	name  string
	order *[]string
	mu    *sync.Mutex
}

func (h *orderHook) BeforeQuery(ctx context.Context, event *sqlex.QueryEvent) context.Context {
	h.mu.Lock()
	defer h.mu.Unlock()
	*h.order = append(*h.order, "before:"+h.name)
	return ctx
}

func (h *orderHook) AfterQuery(ctx context.Context, event *sqlex.QueryEvent) {
	h.mu.Lock()
	defer h.mu.Unlock()
	*h.order = append(*h.order, "after:"+h.name)
}

// pgSchema
// pgSchema is the PostgreSQL-specific test schema (uses SERIAL instead of AUTOINCREMENT)
var pgSchema = Schema{
	Create: `
CREATE TABLE pg_users (
	id SERIAL PRIMARY KEY,
	name TEXT NOT NULL,
	email TEXT NOT NULL,
	age INTEGER DEFAULT 0,
	created_at TIMESTAMP DEFAULT now()
);

CREATE TABLE pg_orders (
	id SERIAL PRIMARY KEY,
	user_id INTEGER NOT NULL,
	amount REAL NOT NULL,
	status VARCHAR(50) DEFAULT 'pending'
);
`,
	Drop: `
DROP TABLE IF EXISTS pg_users;
DROP TABLE IF EXISTS pg_orders;
`,
}

// seedPGData inserts test seed data into PostgreSQL
func seedPGData(db *sqlex.DB, t *testing.T) {
	t.Helper()
	tx := db.MustBegin()
	tx.MustExec(`INSERT INTO pg_users (name, email, age) VALUES ('Alice', 'alice@example.com', 30)`)
	tx.MustExec(`INSERT INTO pg_users (name, email, age) VALUES ('Bob', 'bob@example.com', 25)`)
	tx.MustExec(`INSERT INTO pg_users (name, email, age) VALUES ('Charlie', 'charlie@example.com', 35)`)
	tx.MustExec(`INSERT INTO pg_orders (user_id, amount, status) VALUES (1, 99.9, 'paid')`)
	tx.MustExec(`INSERT INTO pg_orders (user_id, amount, status) VALUES (1, 50.0, 'pending')`)
	tx.MustExec(`INSERT INTO pg_orders (user_id, amount, status) VALUES (2, 200.0, 'paid')`)
	tx.Commit()
}

// ========================================================
// 1. Database connectivity tests
// ========================================================

// TestPGConnectivity verifies PostgreSQL connection and basic operations
func TestPGConnectivity(t *testing.T) {
	pgOnly(t)

	t.Run("Ping", func(t *testing.T) {
		err := pgdb.Ping()
		if err != nil {
			t.Fatalf("Ping failed: %v", err)
		}
	})

	t.Run("PingContext", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		err := pgdb.PingContext(ctx)
		if err != nil {
			t.Fatalf("PingContext failed: %v", err)
		}
	})

	t.Run("DriverName", func(t *testing.T) {
		if pgdb.DriverName() != "postgres" {
			t.Errorf("expected driver name 'postgres', got '%s'", pgdb.DriverName())
		}
	})

	t.Run("Stats", func(t *testing.T) {
		stats := pgdb.Stats()
		// should have at least one open connection
		if stats.OpenConnections <= 0 {
			t.Logf("Warning: OpenConnections = %d (may be 0 in idle pool)", stats.OpenConnections)
		}
		t.Logf("PostgreSQL pool status: Open=%d, InUse=%d, Idle=%d",
			stats.OpenConnections, stats.InUse, stats.Idle)
	})

	t.Run("ServerVersion", func(t *testing.T) {
		var version string
		err := pgdb.Get(&version, "SELECT version()")
		if err != nil {
			t.Fatalf("get PostgreSQL version failed: %v", err)
		}
		t.Logf("PostgreSQL version: %s", version)
	})
}

// ========================================================
// 2. Standard feature tests
// ========================================================

// TestPGBasicCRUD basic CRUD operations
func TestPGBasicCRUD(t *testing.T) {
	pgOnly(t)

	create, drop, _ := pgSchema.Postgres()
	defer multiExec(pgdb, drop)
	multiExec(pgdb, create)
	seedPGData(pgdb, t)

	t.Run("Select", func(t *testing.T) {
		var users []IntUser
		err := pgdb.Select(&users, "SELECT * FROM pg_users ORDER BY id")
		if err != nil {
			t.Fatalf("Select failed: %v", err)
		}
		if len(users) != 3 {
			t.Fatalf("expected 3 users, got %d", len(users))
		}
		if users[0].Name != "Alice" || users[1].Name != "Bob" || users[2].Name != "Charlie" {
			t.Errorf("unexpected user names: %v", users)
		}
	})

	t.Run("Get", func(t *testing.T) {
		var user IntUser
		err := pgdb.Get(&user, "SELECT * FROM pg_users WHERE name = ?", "Alice")
		if err != nil {
			t.Fatalf("Get failed: %v", err)
		}
		if user.Email != "alice@example.com" || user.Age != 30 {
			t.Errorf("unexpected user: %+v", user)
		}
	})

	t.Run("Get_NoRows", func(t *testing.T) {
		var user IntUser
		err := pgdb.Get(&user, "SELECT * FROM pg_users WHERE name = ?", "NonExistent")
		if !errors.Is(err, sql.ErrNoRows) {
			t.Errorf("expected sql.ErrNoRows, got %v", err)
		}
	})

	t.Run("Update", func(t *testing.T) {
		result, err := pgdb.Exec("UPDATE pg_users SET age = ? WHERE name = ?", 31, "Alice")
		if err != nil {
			t.Fatalf("Update failed: %v", err)
		}
		affected, _ := result.RowsAffected()
		if affected != 1 {
			t.Errorf("expected 1 row affected, got %d", affected)
		}

		var user IntUser
		pgdb.Get(&user, "SELECT * FROM pg_users WHERE name = ?", "Alice")
		if user.Age != 31 {
			t.Errorf("expected age 31 after update, got %d", user.Age)
		}
	})

	t.Run("Delete", func(t *testing.T) {
		result, err := pgdb.Exec("DELETE FROM pg_users WHERE name = ?", "Charlie")
		if err != nil {
			t.Fatalf("Delete failed: %v", err)
		}
		affected, _ := result.RowsAffected()
		if affected != 1 {
			t.Errorf("expected 1 row deleted, got %d", affected)
		}

		var count int
		pgdb.Get(&count, "SELECT COUNT(*) FROM pg_users")
		if count != 2 {
			t.Errorf("expected 2 users after delete, got %d", count)
		}
	})

	t.Run("Insert_RETURNING", func(t *testing.T) {
		// PostgreSQL-specific: INSERT ... RETURNING id
		var newID int
		err := pgdb.Get(&newID, "INSERT INTO pg_users (name, email, age) VALUES (?, ?, ?) RETURNING id",
			"Dave", "dave@example.com", 28)
		if err != nil {
			t.Fatalf("INSERT RETURNING failed: %v", err)
		}
		if newID <= 0 {
			t.Errorf("expected positive ID from RETURNING, got %d", newID)
		}
	})
}

// TestPGTransaction transaction handling (rollback and commit)
func TestPGTransaction(t *testing.T) {
	pgOnly(t)

	create, drop, _ := pgSchema.Postgres()
	defer multiExec(pgdb, drop)
	multiExec(pgdb, create)

	t.Run("Commit", func(t *testing.T) {
		tx, err := pgdb.Beginx()
		if err != nil {
			t.Fatalf("Beginx failed: %v", err)
		}
		_, err = tx.Exec("INSERT INTO pg_users (name, email, age) VALUES (?, ?, ?)", "TxUser", "tx@test.com", 20)
		if err != nil {
			tx.Rollback()
			t.Fatalf("tx.Exec failed: %v", err)
		}
		err = tx.Commit()
		if err != nil {
			t.Fatalf("Commit failed: %v", err)
		}

		var user IntUser
		err = pgdb.Get(&user, "SELECT * FROM pg_users WHERE name = ?", "TxUser")
		if err != nil {
			t.Fatalf("Committed user not found: %v", err)
		}
	})

	t.Run("Rollback", func(t *testing.T) {
		tx, err := pgdb.Beginx()
		if err != nil {
			t.Fatalf("Beginx failed: %v", err)
		}
		_, err = tx.Exec("INSERT INTO pg_users (name, email, age) VALUES (?, ?, ?)", "RollbackUser", "rb@test.com", 22)
		if err != nil {
			tx.Rollback()
			t.Fatalf("tx.Exec failed: %v", err)
		}
		tx.Rollback()

		var user IntUser
		err = pgdb.Get(&user, "SELECT * FROM pg_users WHERE name = ?", "RollbackUser")
		if !errors.Is(err, sql.ErrNoRows) {
			t.Errorf("expected RollbackUser to be rolled back, got: %v", err)
		}
	})

	t.Run("CloseWithErr_Commit", func(t *testing.T) {
		func() {
			var err error
			tx, txErr := pgdb.Beginx()
			if txErr != nil {
				t.Fatalf("Beginx failed: %v", txErr)
			}
			defer func() { tx.CloseWithErr(err) }()

			_, err = tx.Exec("INSERT INTO pg_users (name, email, age) VALUES (?, ?, ?)",
				"AutoCommit", "autocommit@test.com", 33)
		}()

		var user IntUser
		err := pgdb.Get(&user, "SELECT * FROM pg_users WHERE name = ?", "AutoCommit")
		if err != nil {
			t.Fatalf("CloseWithErr should have committed: %v", err)
		}
	})

	t.Run("CloseWithErr_Rollback", func(t *testing.T) {
		func() {
			var err error
			tx, txErr := pgdb.Beginx()
			if txErr != nil {
				t.Fatalf("Beginx failed: %v", txErr)
			}
			defer func() { tx.CloseWithErr(err) }()

			_, err = tx.Exec("INSERT INTO pg_users (name, email, age) VALUES (?, ?, ?)",
				"AutoRollback", "autorollback@test.com", 33)
			if err != nil {
				return
			}
			err = fmt.Errorf("business error")
		}()

		var user IntUser
		err := pgdb.Get(&user, "SELECT * FROM pg_users WHERE name = ?", "AutoRollback")
		if !errors.Is(err, sql.ErrNoRows) {
			t.Errorf("CloseWithErr should have rolled back: %v", err)
		}
	})
}

// TestPGPreparedStatement prepared statement tests
func TestPGPreparedStatement(t *testing.T) {
	pgOnly(t)

	create, drop, _ := pgSchema.Postgres()
	defer multiExec(pgdb, drop)
	multiExec(pgdb, create)
	seedPGData(pgdb, t)

	t.Run("Preparex", func(t *testing.T) {
		// Use ? placeholder uniformly; Preparex auto-Rebinds to $1
		stmt, err := pgdb.Preparex("SELECT * FROM pg_users WHERE name = ?")
		if err != nil {
			t.Fatalf("Preparex failed: %v", err)
		}
		defer stmt.Close()

		var user IntUser
		err = stmt.Get(&user, "Alice")
		if err != nil {
			t.Fatalf("stmt.Get failed: %v", err)
		}
		if user.Email != "alice@example.com" {
			t.Errorf("expected alice@example.com, got %s", user.Email)
		}
	})

	t.Run("PrepareNamed", func(t *testing.T) {
		ns, err := pgdb.PrepareNamed(`INSERT INTO pg_users (name, email, age) VALUES (:name, :email, :age)`)
		if err != nil {
			t.Fatalf("PrepareNamed failed: %v", err)
		}
		defer ns.Close()

		result, err := ns.Exec(map[string]any{"name": "StmtTest", "email": "stmt@test.com", "age": 40})
		if err != nil {
			t.Fatalf("NamedStmt.Exec failed: %v", err)
		}
		if result == nil {
			t.Fatal("expected non-nil sql.Result")
		}
		rowsAff, _ := result.RowsAffected()
		if rowsAff != 1 {
			t.Errorf("expected 1 row affected, got %d", rowsAff)
		}
	})

	t.Run("PreparedSelect", func(t *testing.T) {
		// Use ? placeholder uniformly; Preparex auto-Rebinds
		stmt, err := pgdb.Preparex("SELECT * FROM pg_users WHERE age > ? ORDER BY age")
		if err != nil {
			t.Fatalf("Preparex failed: %v", err)
		}
		defer stmt.Close()

		var users []IntUser
		err = stmt.Select(&users, 26)
		if err != nil {
			t.Fatalf("stmt.Select failed: %v", err)
		}
		if len(users) < 2 {
			t.Errorf("expected at least 2 users with age > 26, got %d", len(users))
		}
	})
}

// ========================================================
// 3. New feature tests
// ========================================================

// TestPGHook verifies Hook mechanism integrity in PostgreSQL
func TestPGHook(t *testing.T) {
	pgOnly(t)

	create, drop, _ := pgSchema.Postgres()
	defer multiExec(pgdb, drop)
	multiExec(pgdb, create)

	t.Run("BasicHook", func(t *testing.T) {
		// Create independent DB wrapper to avoid polluting global pgdb hooks
		db := sqlex.NewDb(pgdb.DB, pgdb.DriverName())
		hook := &testHook{}
		db.AddHook(hook)

		ctx := context.Background()
		_, err := db.ExecContext(ctx, "INSERT INTO pg_users (name, email, age) VALUES (?, ?, ?)", "HookUser", "hook@test.com", 20)
		if err != nil {
			t.Fatalf("ExecContext failed: %v", err)
		}

		var users []IntUser
		err = db.SelectContext(ctx, &users, "SELECT * FROM pg_users ORDER BY id")
		if err != nil {
			t.Fatalf("SelectContext failed: %v", err)
		}

		hook.mu.Lock()
		defer hook.mu.Unlock()

		if hook.beforeCount == 0 {
			t.Error("expected BeforeQuery to be called")
		}
		if hook.afterCount == 0 {
			t.Error("expected AfterQuery to be called")
		}

		// Verify the query was recorded
		found := false
		for _, q := range hook.queries {
			if q == "SELECT * FROM pg_users ORDER BY id" {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected SELECT query in hook records, got: %v", hook.queries)
		}
	})

	t.Run("OnionModel", func(t *testing.T) {
		db := sqlex.NewDb(pgdb.DB, pgdb.DriverName())
		var order []string
		var mu sync.Mutex

		hook1 := &orderHook{name: "A", order: &order, mu: &mu}
		hook2 := &orderHook{name: "B", order: &order, mu: &mu}
		db.AddHook(hook1)
		db.AddHook(hook2)

		ctx := context.Background()
		var dummy IntUser
		db.GetContext(ctx, &dummy, "SELECT * FROM pg_users LIMIT 1")

		mu.Lock()
		defer mu.Unlock()

		if len(order) < 4 {
			t.Fatalf("expected at least 4 hook calls, got %d: %v", len(order), order)
		}
		last4 := order[len(order)-4:]
		expected := []string{"before:A", "before:B", "after:B", "after:A"}
		for i, exp := range expected {
			if last4[i] != exp {
				t.Errorf("hook order[%d]: expected %s, got %s (full: %v)", i, exp, last4[i], order)
			}
		}
	})

	t.Run("HookInheritedByTx", func(t *testing.T) {
		db := sqlex.NewDb(pgdb.DB, pgdb.DriverName())
		hook := &testHook{}
		db.AddHook(hook)

		tx, err := db.Beginx()
		if err != nil {
			t.Fatalf("Beginx failed: %v", err)
		}
		defer tx.Rollback()

		ctx := context.Background()
		_, _ = tx.ExecContext(ctx, "INSERT INTO pg_users (name, email, age) VALUES (?, ?, ?)", "TxHookPG", "txhookpg@test.com", 10)
		var dummy IntUser
		tx.GetContext(ctx, &dummy, "SELECT * FROM pg_users WHERE name = ?", "TxHookPG")

		hook.mu.Lock()
		defer hook.mu.Unlock()

		// Hook records query in Rebind format ($N), because Rebind runs before Hook
		found := false
		for _, q := range hook.queries {
			if q == "SELECT * FROM pg_users WHERE name = $1" {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected tx query to trigger hook, got queries: %v", hook.queries)
		}
	})

	t.Run("HookCapturesError", func(t *testing.T) {
		db := sqlex.NewDb(pgdb.DB, pgdb.DriverName())

		var capturedError error
		var capturedDuration time.Duration
		errorHook := &funcHook{
			before: func(ctx context.Context, event *sqlex.QueryEvent) context.Context { return ctx },
			after: func(ctx context.Context, event *sqlex.QueryEvent) {
				capturedError = event.Error
				capturedDuration = event.Duration
			},
		}
		db.AddHook(errorHook)

		ctx := context.Background()
		// Execute a query that will fail
		_, _ = db.ExecContext(ctx, "SELECT * FROM nonexistent_table_xxx")

		if capturedError == nil {
			t.Error("expected hook to capture error from failed query")
		}
		if capturedDuration == 0 {
			t.Error("expected hook to capture non-zero duration")
		}
	})
}

// funcHook is a function-based Hook implementation for testing
type funcHook struct {
	before func(ctx context.Context, event *sqlex.QueryEvent) context.Context
	after  func(ctx context.Context, event *sqlex.QueryEvent)
}

func (h *funcHook) BeforeQuery(ctx context.Context, event *sqlex.QueryEvent) context.Context {
	return h.before(ctx, event)
}

func (h *funcHook) AfterQuery(ctx context.Context, event *sqlex.QueryEvent) {
	h.after(ctx, event)
}

// TestPGJsonValue tests JsonValue[T] mapping with PostgreSQL JSON columns
func TestPGJsonValue(t *testing.T) {
	pgOnly(t)

	jsonSchema := Schema{
		Create: `
CREATE TABLE pg_json_test (
	id SERIAL PRIMARY KEY,
	name TEXT NOT NULL,
	metadata JSON,
	settings JSONB,
	tags JSONB
);`,
		Drop: `DROP TABLE IF EXISTS pg_json_test;`,
	}

	create, drop, _ := jsonSchema.Postgres()
	defer multiExec(pgdb, drop)
	multiExec(pgdb, create)

	// Define test structs
	type Metadata struct {
		Version string `json:"version"`
		Author  string `json:"author"`
	}

	type Settings struct {
		Theme    string `json:"theme"`
		FontSize int    `json:"font_size"`
		Debug    bool   `json:"debug"`
	}

	type JSONRow struct {
		ID       int                       `db:"id"`
		Name     string                    `db:"name"`
		Metadata types.JsonValue[Metadata] `db:"metadata"`
		Settings types.JsonValue[Settings] `db:"settings"`
		Tags     types.JsonValue[[]string] `db:"tags"`
	}

	t.Run("Insert_And_Read_JSON", func(t *testing.T) {
		row := JSONRow{
			Name:     "test1",
			Metadata: types.NewJsonValue(Metadata{Version: "1.0", Author: "Alice"}),
			Settings: types.NewJsonValue(Settings{Theme: "dark", FontSize: 14, Debug: true}),
			Tags:     types.NewJsonValue([]string{"go", "postgres", "json"}),
		}

		_, err := pgdb.NamedExec(`INSERT INTO pg_json_test (name, metadata, settings, tags) 
			VALUES (:name, :metadata, :settings, :tags)`, row)
		if err != nil {
			t.Fatalf("Insert JSON row failed: %v", err)
		}

		var result JSONRow
		err = pgdb.Get(&result, "SELECT * FROM pg_json_test WHERE name = ?", "test1")
		if err != nil {
			t.Fatalf("Get JSON row failed: %v", err)
		}

		if !result.Metadata.Valid {
			t.Fatal("expected Metadata to be valid")
		}
		if result.Metadata.Val.Version != "1.0" || result.Metadata.Val.Author != "Alice" {
			t.Errorf("unexpected Metadata: %+v", result.Metadata.Val)
		}

		if !result.Settings.Valid {
			t.Fatal("expected Settings to be valid")
		}
		if result.Settings.Val.Theme != "dark" || result.Settings.Val.FontSize != 14 || !result.Settings.Val.Debug {
			t.Errorf("unexpected Settings: %+v", result.Settings.Val)
		}

		if !result.Tags.Valid || len(result.Tags.Val) != 3 {
			t.Fatalf("expected 3 tags, got %+v", result.Tags)
		}
		if result.Tags.Val[0] != "go" || result.Tags.Val[1] != "postgres" || result.Tags.Val[2] != "json" {
			t.Errorf("unexpected Tags: %v", result.Tags.Val)
		}
	})

	t.Run("Null_JSON", func(t *testing.T) {
		_, err := pgdb.Exec("INSERT INTO pg_json_test (name) VALUES (?)", "nulljson")
		if err != nil {
			t.Fatalf("Insert null JSON failed: %v", err)
		}

		var result JSONRow
		err = pgdb.Get(&result, "SELECT * FROM pg_json_test WHERE name = ?", "nulljson")
		if err != nil {
			t.Fatalf("Get null JSON row failed: %v", err)
		}

		if result.Metadata.Valid {
			t.Error("expected Metadata to be invalid for NULL")
		}
		if result.Settings.Valid {
			t.Error("expected Settings to be invalid for NULL")
		}
		if result.Tags.Valid {
			t.Error("expected Tags to be invalid for NULL")
		}
	})

	t.Run("JsonValue_MarshalJSON", func(t *testing.T) {
		jv := types.NewJsonValue(Metadata{Version: "2.0", Author: "Bob"})
		data, err := json.Marshal(jv)
		if err != nil {
			t.Fatalf("MarshalJSON failed: %v", err)
		}
		expected := `{"version":"2.0","author":"Bob"}`
		if string(data) != expected {
			t.Errorf("expected %s, got %s", expected, string(data))
		}

		// Test invalid value serialization
		var invalid types.JsonValue[Metadata]
		data, err = json.Marshal(invalid)
		if err != nil {
			t.Fatalf("MarshalJSON for invalid failed: %v", err)
		}
		if string(data) != "null" {
			t.Errorf("expected 'null', got %s", string(data))
		}
	})

	t.Run("JsonValue_Map", func(t *testing.T) {
		// Test JsonValue with map type
		type MapRow struct {
			ID       int                             `db:"id"`
			Name     string                          `db:"name"`
			Settings types.JsonValue[map[string]any] `db:"settings"`
		}

		_, err := pgdb.Exec(`INSERT INTO pg_json_test (name, settings) VALUES (?, ?)`,
			"mapjson", `{"key1":"val1","key2":42}`)
		if err != nil {
			t.Fatalf("Insert map JSON failed: %v", err)
		}

		var result MapRow
		err = pgdb.Get(&result, "SELECT id, name, settings FROM pg_json_test WHERE name = ?", "mapjson")
		if err != nil {
			t.Fatalf("Get map JSON failed: %v", err)
		}

		if !result.Settings.Valid {
			t.Fatal("expected Settings to be valid")
		}
		if result.Settings.Val["key1"] != "val1" {
			t.Errorf("expected key1=val1, got %v", result.Settings.Val["key1"])
		}
	})

	t.Run("JSONB_Query_Operators", func(t *testing.T) {
		// Test JSONB query operators
		_, err := pgdb.Exec(`INSERT INTO pg_json_test (name, settings) VALUES (?, ?::jsonb)`,
			"jsonbquery", `{"theme":"light","font_size":16}`)
		if err != nil {
			t.Fatalf("Insert JSONB failed: %v", err)
		}

		// Use ->> operator to query JSONB field
		var theme string
		err = pgdb.Get(&theme, "SELECT settings->>'theme' FROM pg_json_test WHERE name = ?", "jsonbquery")
		if err != nil {
			t.Fatalf("JSONB ->> query failed: %v", err)
		}
		if theme != "light" {
			t.Errorf("expected theme 'light', got '%s'", theme)
		}

		// Use @> operator (JSONB containment query)
		var name string
		err = pgdb.Get(&name, `SELECT name FROM pg_json_test WHERE settings @> '{"theme":"light"}'::jsonb AND name = ?`, "jsonbquery")
		if err != nil {
			t.Fatalf("JSONB @> query failed: %v", err)
		}
		if name != "jsonbquery" {
			t.Errorf("expected 'jsonbquery', got '%s'", name)
		}
	})
}

// TestPGNamedQuery Named 查询功能在 PostgreSQL 中的支持情况
func TestPGNamedQuery(t *testing.T) {
	pgOnly(t)

	create, drop, _ := pgSchema.Postgres()
	defer multiExec(pgdb, drop)
	multiExec(pgdb, create)
	seedPGData(pgdb, t)

	t.Run("NamedGet", func(t *testing.T) {
		var user IntUser
		err := pgdb.NamedGet(&user, `SELECT * FROM pg_users WHERE name = :name`,
			map[string]any{"name": "Alice"})
		if err != nil {
			t.Fatalf("NamedGet failed: %v", err)
		}
		if user.Email != "alice@example.com" {
			t.Errorf("expected alice@example.com, got %s", user.Email)
		}
	})

	t.Run("NamedGet_NoRows", func(t *testing.T) {
		var user IntUser
		err := pgdb.NamedGet(&user, `SELECT * FROM pg_users WHERE name = :name`,
			map[string]any{"name": "Nobody"})
		if !errors.Is(err, sql.ErrNoRows) {
			t.Errorf("expected sql.ErrNoRows, got %v", err)
		}
	})

	t.Run("NamedSelect", func(t *testing.T) {
		var users []IntUser
		err := pgdb.NamedSelect(&users, `SELECT * FROM pg_users WHERE age > :min_age ORDER BY age`,
			map[string]any{"min_age": 26})
		if err != nil {
			t.Fatalf("NamedSelect failed: %v", err)
		}
		if len(users) != 2 {
			t.Fatalf("expected 2 users, got %d", len(users))
		}
		if users[0].Name != "Alice" || users[1].Name != "Charlie" {
			t.Errorf("unexpected order: %v, %v", users[0].Name, users[1].Name)
		}
	})

	t.Run("NamedSelect_Struct", func(t *testing.T) {
		type AgeFilter struct {
			MinAge int `db:"min_age"`
		}
		var users []IntUser
		err := pgdb.NamedSelect(&users, `SELECT * FROM pg_users WHERE age > :min_age`,
			AgeFilter{MinAge: 30})
		if err != nil {
			t.Fatalf("NamedSelect with struct failed: %v", err)
		}
		if len(users) != 1 || users[0].Name != "Charlie" {
			t.Errorf("expected [Charlie], got %v", users)
		}
	})

	t.Run("NamedSelect_IN", func(t *testing.T) {
		var users []IntUser
		err := pgdb.NamedSelect(&users,
			`SELECT * FROM pg_users WHERE name IN (:names) ORDER BY name`,
			map[string]any{"names": []string{"Alice", "Charlie"}})
		if err != nil {
			t.Fatalf("NamedSelect with IN failed: %v", err)
		}
		if len(users) != 2 {
			t.Fatalf("expected 2 users, got %d", len(users))
		}
	})

	t.Run("NamedExec", func(t *testing.T) {
		result, err := pgdb.NamedExec(
			`INSERT INTO pg_users (name, email, age) VALUES (:name, :email, :age)`,
			map[string]any{"name": "NamedUser", "email": "named@test.com", "age": 45})
		if err != nil {
			t.Fatalf("NamedExec failed: %v", err)
		}
		affected, _ := result.RowsAffected()
		if affected != 1 {
			t.Errorf("expected 1 row affected, got %d", affected)
		}
	})

	t.Run("NamedExecContext_IN", func(t *testing.T) {
		ctx := context.Background()
		result, err := pgdb.NamedExecContext(ctx,
			`DELETE FROM pg_orders WHERE user_id IN (:user_ids)`,
			map[string]any{"user_ids": []int{1, 2}})
		if err != nil {
			t.Fatalf("NamedExecContext with IN failed: %v", err)
		}
		affected, _ := result.RowsAffected()
		if affected != 3 {
			t.Errorf("expected 3 rows affected, got %d", affected)
		}
	})

	// Note: sqlex does not support Unicode param names, but Unicode data values are fine.
	// Use ASCII param names + Unicode values to verify real scenarios.
	t.Run("NamedQuery_UnicodeValue", func(t *testing.T) {
		_, err := pgdb.NamedExec(
			`INSERT INTO pg_users (name, email, age) VALUES (:name, :email, :age)`,
			map[string]any{"name": "太郎", "email": "taro@example.com", "age": 28})
		if err != nil {
			t.Fatalf("NamedExec with Unicode values failed: %v", err)
		}

		var user IntUser
		err = pgdb.Get(&user, "SELECT * FROM pg_users WHERE name = ?", "太郎")
		if err != nil {
			t.Fatalf("Get Unicode user failed: %v", err)
		}
		if user.Email != "taro@example.com" || user.Age != 28 {
			t.Errorf("unexpected user: %+v", user)
		}
	})

	t.Run("Tx_NamedOperations", func(t *testing.T) {
		tx, err := pgdb.Beginx()
		if err != nil {
			t.Fatalf("Beginx failed: %v", err)
		}
		defer tx.Rollback()

		// Tx.NamedGet
		var user IntUser
		err = tx.NamedGet(&user, `SELECT * FROM pg_users WHERE email = :email`,
			map[string]any{"email": "alice@example.com"})
		if err != nil {
			t.Fatalf("Tx.NamedGet failed: %v", err)
		}
		if user.Name != "Alice" {
			t.Errorf("expected Alice, got %s", user.Name)
		}

		// Tx.NamedSelect
		var users []IntUser
		err = tx.NamedSelect(&users, `SELECT * FROM pg_users WHERE age < :max_age ORDER BY age`,
			map[string]any{"max_age": 32})
		if err != nil {
			t.Fatalf("Tx.NamedSelect failed: %v", err)
		}
		if len(users) < 2 {
			t.Errorf("expected at least 2 users, got %d", len(users))
		}

		// Tx.NamedSelect with IN
		var selected []IntUser
		err = tx.NamedSelect(&selected,
			`SELECT * FROM pg_users WHERE name IN (:names) ORDER BY name`,
			map[string]any{"names": []string{"Alice", "Bob"}})
		if err != nil {
			t.Fatalf("Tx.NamedSelect with IN failed: %v", err)
		}
		if len(selected) != 2 {
			t.Errorf("expected 2 users, got %d", len(selected))
		}
	})
}

// TestPGContext tests context propagation and timeout control
func TestPGContext(t *testing.T) {
	pgOnly(t)

	create, drop, _ := pgSchema.Postgres()
	defer multiExec(pgdb, drop)
	multiExec(pgdb, create)
	seedPGData(pgdb, t)

	t.Run("SelectContext", func(t *testing.T) {
		ctx := context.Background()
		var users []IntUser
		err := pgdb.SelectContext(ctx, &users, "SELECT * FROM pg_users ORDER BY id")
		if err != nil {
			t.Fatalf("SelectContext failed: %v", err)
		}
		if len(users) != 3 {
			t.Errorf("expected 3 users, got %d", len(users))
		}
	})

	t.Run("GetContext", func(t *testing.T) {
		ctx := context.Background()
		var user IntUser
		err := pgdb.GetContext(ctx, &user, "SELECT * FROM pg_users WHERE name = ?", "Bob")
		if err != nil {
			t.Fatalf("GetContext failed: %v", err)
		}
		if user.Age != 25 {
			t.Errorf("expected age 25, got %d", user.Age)
		}
	})

	t.Run("NamedGetContext", func(t *testing.T) {
		ctx := context.Background()
		var user IntUser
		err := pgdb.NamedGetContext(ctx, &user, `SELECT * FROM pg_users WHERE name = :name`,
			map[string]any{"name": "Charlie"})
		if err != nil {
			t.Fatalf("NamedGetContext failed: %v", err)
		}
		if user.Age != 35 {
			t.Errorf("expected age 35, got %d", user.Age)
		}
	})

	t.Run("NamedSelectContext", func(t *testing.T) {
		ctx := context.Background()
		var users []IntUser
		err := pgdb.NamedSelectContext(ctx, &users, `SELECT * FROM pg_users WHERE age >= :age ORDER BY age`,
			map[string]any{"age": 30})
		if err != nil {
			t.Fatalf("NamedSelectContext failed: %v", err)
		}
		if len(users) != 2 {
			t.Errorf("expected 2 users, got %d", len(users))
		}
	})

	t.Run("NamedSelectContext_IN", func(t *testing.T) {
		ctx := context.Background()
		var users []IntUser
		err := pgdb.NamedSelectContext(ctx, &users,
			`SELECT * FROM pg_users WHERE name IN (:names)`,
			map[string]any{"names": []string{"Alice", "Bob"}})
		if err != nil {
			t.Fatalf("NamedSelectContext with IN failed: %v", err)
		}
		if len(users) != 2 {
			t.Errorf("expected 2 users, got %d", len(users))
		}
	})

	t.Run("ContextTimeout", func(t *testing.T) {
		// Use very short timeout to test Context cancellation
		ctx, cancel := context.WithTimeout(context.Background(), 1*time.Nanosecond)
		defer cancel()
		// Give some time for the context to expire
		time.Sleep(1 * time.Millisecond)

		var users []IntUser
		err := pgdb.SelectContext(ctx, &users, "SELECT * FROM pg_users")
		if err == nil {
			t.Error("expected timeout error, got nil")
		}
	})

	t.Run("ContextCancel", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel() // immediately cancel

		var users []IntUser
		err := pgdb.SelectContext(ctx, &users, "SELECT * FROM pg_users")
		if err == nil {
			t.Error("expected cancel error, got nil")
		}
	})
}

// ========================================================
// 4. PostgreSQL-specific feature tests
// ========================================================

// TestPGJSONBTypes tests JSON/JSONB data type support
func TestPGJSONBTypes(t *testing.T) {
	pgOnly(t)

	schema := Schema{
		Create: `
CREATE TABLE pg_jsonb_advanced (
	id SERIAL PRIMARY KEY,
	data JSONB NOT NULL DEFAULT '{}'::jsonb,
	nested JSONB
);`,
		Drop: `DROP TABLE IF EXISTS pg_jsonb_advanced;`,
	}

	create, drop, _ := schema.Postgres()
	defer multiExec(pgdb, drop)
	multiExec(pgdb, create)

	t.Run("DeepNestedJSON", func(t *testing.T) {
		deepData := map[string]any{
			"level1": map[string]any{
				"level2": map[string]any{
					"level3": "deep_value",
				},
			},
			"array": []any{1.0, 2.0, 3.0},
		}
		jsonBytes, _ := json.Marshal(deepData)

		_, err := pgdb.Exec("INSERT INTO pg_jsonb_advanced (data) VALUES (?::jsonb)", string(jsonBytes))
		if err != nil {
			t.Fatalf("Insert deep nested JSON failed: %v", err)
		}

		// Use JSON path query
		var val string
		err = pgdb.Get(&val, "SELECT data->'level1'->'level2'->>'level3' FROM pg_jsonb_advanced LIMIT 1")
		if err != nil {
			t.Fatalf("Deep JSON path query failed: %v", err)
		}
		if val != "deep_value" {
			t.Errorf("expected 'deep_value', got '%s'", val)
		}
	})

	t.Run("JSONB_Contains", func(t *testing.T) {
		_, err := pgdb.Exec(`INSERT INTO pg_jsonb_advanced (data) VALUES ('{"tags":["go","pg"],"active":true}'::jsonb)`)
		if err != nil {
			t.Fatalf("Insert JSONB failed: %v", err)
		}

		var count int
		err = pgdb.Get(&count, `SELECT COUNT(*) FROM pg_jsonb_advanced WHERE data @> '{"active":true}'::jsonb`)
		if err != nil {
			t.Fatalf("JSONB contains query failed: %v", err)
		}
		if count == 0 {
			t.Error("expected at least 1 result from JSONB @> query")
		}
	})

	t.Run("JSONB_Existence", func(t *testing.T) {
		// Using jsonb_exists function to test key existence
		// Note: direct use of ? operator in SQL conflicts with placeholder,
		// use Rebind("data ?? 'key'") or jsonb_exists function instead
		var count int
		err := pgdb.Get(&count, `SELECT COUNT(*) FROM pg_jsonb_advanced WHERE jsonb_exists(data, 'tags')`)
		if err != nil {
			t.Fatalf("JSONB key existence query failed: %v", err)
		}
		if count == 0 {
			t.Error("expected at least 1 result from JSONB key existence query")
		}

		// Also verify via ?? escape (framework auto-Rebinds, ?? outputs literal ?)
		count = 0
		err = pgdb.Get(&count, `SELECT COUNT(*) FROM pg_jsonb_advanced WHERE data ?? 'tags'`)
		if err != nil {
			t.Fatalf("JSONB ?? via auto Rebind failed: %v", err)
		}
		if count == 0 {
			t.Error("expected at least 1 result from JSONB ?? via auto Rebind")
		}
	})
}

// TestPGArrayTypes tests array type support
func TestPGArrayTypes(t *testing.T) {
	pgOnly(t)

	schema := Schema{
		Create: `
CREATE TABLE pg_array_test (
	id SERIAL PRIMARY KEY,
	name TEXT NOT NULL,
	int_arr INTEGER[],
	text_arr TEXT[]
);`,
		Drop: `DROP TABLE IF EXISTS pg_array_test;`,
	}

	create, drop, _ := schema.Postgres()
	defer multiExec(pgdb, drop)
	multiExec(pgdb, create)

	t.Run("Insert_ArrayLiteral", func(t *testing.T) {
		_, err := pgdb.Exec("INSERT INTO pg_array_test (name, int_arr, text_arr) VALUES (?, ?, ?)",
			"test1", "{1,2,3}", "{hello,world}")
		if err != nil {
			t.Fatalf("Insert array literal failed: %v", err)
		}

		// Read and verify
		var intArr, textArr string
		row := pgdb.QueryRowx("SELECT int_arr::text, text_arr::text FROM pg_array_test WHERE name = ?", "test1")
		err = row.Scan(&intArr, &textArr)
		if err != nil {
			t.Fatalf("Read array failed: %v", err)
		}
		if intArr != "{1,2,3}" {
			t.Errorf("expected int_arr {1,2,3}, got %s", intArr)
		}
	})

	t.Run("Array_ANY", func(t *testing.T) {
		_, err := pgdb.Exec("INSERT INTO pg_array_test (name, int_arr) VALUES (?, ?)", "test2", "{10,20,30}")
		if err != nil {
			t.Fatalf("Insert failed: %v", err)
		}

		// Use ANY operator
		var name string
		err = pgdb.Get(&name, "SELECT name FROM pg_array_test WHERE ? = ANY(int_arr)", 20)
		if err != nil {
			t.Fatalf("ANY query failed: %v", err)
		}
		if name != "test2" {
			t.Errorf("expected 'test2', got '%s'", name)
		}
	})

	t.Run("Array_Unnest", func(t *testing.T) {
		var values []int
		err := pgdb.Select(&values, "SELECT unnest(int_arr) FROM pg_array_test WHERE name = ?", "test1")
		if err != nil {
			t.Fatalf("Unnest query failed: %v", err)
		}
		if len(values) != 3 {
			t.Errorf("expected 3 values from unnest, got %d", len(values))
		}
	})
}

// TestPGTypeCasting PostgreSQL :: 类型转换语法兼容性
func TestPGTypeCasting(t *testing.T) {
	pgOnly(t)

	create, drop, _ := pgSchema.Postgres()
	defer multiExec(pgdb, drop)
	multiExec(pgdb, create)
	seedPGData(pgdb, t)

	t.Run("DoubleColon_Cast", func(t *testing.T) {
		// :: 类型转换不应干扰命名参数解析
		var result string
		err := pgdb.Get(&result, "SELECT name::text FROM pg_users WHERE id = ?", 1)
		if err != nil {
			t.Fatalf(":: type cast failed: %v", err)
		}
		if result != "Alice" {
			t.Errorf("expected 'Alice', got '%s'", result)
		}
	})

	t.Run("DoubleColon_WithNamedQuery", func(t *testing.T) {
		var result string
		err := pgdb.NamedGet(&result,
			`SELECT name::text FROM pg_users WHERE age::text = :age_str`,
			map[string]any{"age_str": "30"})
		if err != nil {
			t.Fatalf("Named query with :: failed: %v", err)
		}
		if result != "Alice" {
			t.Errorf("expected 'Alice', got '%s'", result)
		}
	})

	t.Run("NamedParam_WithDoubleColonCast", func(t *testing.T) {
		var result string
		err := pgdb.NamedGet(&result,
			`SELECT name FROM pg_users WHERE id = :user_id::int`,
			map[string]any{"user_id": "1"})
		if err != nil {
			t.Fatalf("Named param with :: cast failed: %v", err)
		}
		if result != "Alice" {
			t.Errorf("expected 'Alice', got '%s'", result)
		}
	})

	t.Run("Cast_Functions", func(t *testing.T) {
		var result string
		err := pgdb.Get(&result, "SELECT CAST(age AS text) FROM pg_users WHERE name = ?", "Alice")
		if err != nil {
			t.Fatalf("CAST query failed: %v", err)
		}
		if result != "30" {
			t.Errorf("expected '30', got '%s'", result)
		}
	})
}

// TestPGSpecificSQLSyntax tests PostgreSQL-specific SQL syntax compatibility
func TestPGSpecificSQLSyntax(t *testing.T) {
	pgOnly(t)

	create, drop, _ := pgSchema.Postgres()
	defer multiExec(pgdb, drop)
	multiExec(pgdb, create)
	seedPGData(pgdb, t)

	t.Run("DollarQuoting", func(t *testing.T) {
		// PostgreSQL supports $$ dollar quoting
		var result string
		err := pgdb.Get(&result, `SELECT $$hello world$$`)
		if err != nil {
			t.Fatalf("Dollar quoting failed: %v", err)
		}
		if result != "hello world" {
			t.Errorf("expected 'hello world', got '%s'", result)
		}
	})

	t.Run("CTE_WithQuery", func(t *testing.T) {
		// Common Table Expression (WITH clause)
		var users []IntUser
		err := pgdb.Select(&users, `
			WITH young_users AS (
				SELECT * FROM pg_users WHERE age < 32
			)
			SELECT * FROM young_users ORDER BY age`)
		if err != nil {
			t.Fatalf("CTE query failed: %v", err)
		}
		if len(users) != 2 {
			t.Errorf("expected 2 users from CTE, got %d", len(users))
		}
	})

	t.Run("WindowFunction", func(t *testing.T) {
		type RankedUser struct {
			Name string `db:"name"`
			Age  int    `db:"age"`
			Rank int    `db:"rank"`
		}
		var ranked []RankedUser
		err := pgdb.Select(&ranked, `
			SELECT name, age, RANK() OVER (ORDER BY age DESC) as rank 
			FROM pg_users`)
		if err != nil {
			t.Fatalf("Window function query failed: %v", err)
		}
		if len(ranked) != 3 {
			t.Errorf("expected 3 ranked users, got %d", len(ranked))
		}
		if ranked[0].Name != "Charlie" || ranked[0].Rank != 1 {
			t.Errorf("expected Charlie as rank 1, got %s rank %d", ranked[0].Name, ranked[0].Rank)
		}
	})

	t.Run("UPSERT_OnConflict", func(t *testing.T) {
		// First add UNIQUE constraint
		pgdb.Exec("CREATE UNIQUE INDEX IF NOT EXISTS pg_users_email_idx ON pg_users (email)")

		// INSERT ... ON CONFLICT DO UPDATE (PostgreSQL UPSERT)
		_, err := pgdb.Exec(`
			INSERT INTO pg_users (name, email, age) VALUES (?, ?, ?) 
			ON CONFLICT (email) DO UPDATE SET age = EXCLUDED.age`,
			"Alice", "alice@example.com", 99)
		if err != nil {
			t.Fatalf("UPSERT failed: %v", err)
		}

		var age int
		pgdb.Get(&age, "SELECT age FROM pg_users WHERE email = ?", "alice@example.com")
		if age != 99 {
			t.Errorf("expected age 99 after UPSERT, got %d", age)
		}
	})

	t.Run("RETURNING_Clause", func(t *testing.T) {
		type UpdatedUser struct {
			ID  int `db:"id"`
			Age int `db:"age"`
		}
		var updated []UpdatedUser
		err := pgdb.Select(&updated, `
			UPDATE pg_users SET age = age + 1 WHERE age < 40 RETURNING id, age`)
		if err != nil {
			t.Fatalf("UPDATE RETURNING failed: %v", err)
		}
		if len(updated) == 0 {
			t.Error("expected at least 1 updated row with RETURNING")
		}
	})
}

// ========================================================
// 5. Edge cases and error handling
// ========================================================

// TestPGNullHandling tests null value handling
func TestPGNullHandling(t *testing.T) {
	pgOnly(t)

	schema := Schema{
		Create: `
CREATE TABLE pg_null_test (
	id SERIAL PRIMARY KEY,
	name TEXT NOT NULL,
	nullable_text TEXT,
	nullable_int INTEGER,
	nullable_float DOUBLE PRECISION,
	nullable_bool BOOLEAN,
	nullable_time TIMESTAMP
);`,
		Drop: `DROP TABLE IF EXISTS pg_null_test;`,
	}

	create, drop, _ := schema.Postgres()
	defer multiExec(pgdb, drop)
	multiExec(pgdb, create)

	type NullRow struct {
		ID            int             `db:"id"`
		Name          string          `db:"name"`
		NullableText  sql.NullString  `db:"nullable_text"`
		NullableInt   sql.NullInt64   `db:"nullable_int"`
		NullableFloat sql.NullFloat64 `db:"nullable_float"`
		NullableBool  sql.NullBool    `db:"nullable_bool"`
		NullableTime  sql.NullTime    `db:"nullable_time"`
	}

	t.Run("AllNulls", func(t *testing.T) {
		_, err := pgdb.Exec("INSERT INTO pg_null_test (name) VALUES (?)", "allnulls")
		if err != nil {
			t.Fatalf("Insert all nulls failed: %v", err)
		}

		var row NullRow
		err = pgdb.Get(&row, "SELECT * FROM pg_null_test WHERE name = ?", "allnulls")
		if err != nil {
			t.Fatalf("Get all nulls failed: %v", err)
		}

		if row.NullableText.Valid || row.NullableInt.Valid || row.NullableFloat.Valid ||
			row.NullableBool.Valid || row.NullableTime.Valid {
			t.Error("expected all nullable fields to be invalid")
		}
	})

	t.Run("AllValues", func(t *testing.T) {
		now := time.Now().Truncate(time.Microsecond) // PostgreSQL microsecond precision
		_, err := pgdb.Exec(`INSERT INTO pg_null_test (name, nullable_text, nullable_int, nullable_float, nullable_bool, nullable_time) 
			VALUES (?, ?, ?, ?, ?, ?)`,
			"allvalues", "hello", 42, 3.14, true, now)
		if err != nil {
			t.Fatalf("Insert all values failed: %v", err)
		}

		var row NullRow
		err = pgdb.Get(&row, "SELECT * FROM pg_null_test WHERE name = ?", "allvalues")
		if err != nil {
			t.Fatalf("Get all values failed: %v", err)
		}

		if !row.NullableText.Valid || row.NullableText.String != "hello" {
			t.Errorf("expected nullable_text 'hello', got %+v", row.NullableText)
		}
		if !row.NullableInt.Valid || row.NullableInt.Int64 != 42 {
			t.Errorf("expected nullable_int 42, got %+v", row.NullableInt)
		}
		if !row.NullableFloat.Valid {
			t.Errorf("expected nullable_float to be valid")
		}
		if !row.NullableBool.Valid || !row.NullableBool.Bool {
			t.Errorf("expected nullable_bool true, got %+v", row.NullableBool)
		}
		if !row.NullableTime.Valid {
			t.Error("expected nullable_time to be valid")
		}
	})

	t.Run("PointerNull", func(t *testing.T) {
		type PtrRow struct {
			ID   int     `db:"id"`
			Name string  `db:"name"`
			Text *string `db:"nullable_text"`
			Num  *int    `db:"nullable_int"`
		}

		var row PtrRow
		err := pgdb.Get(&row, "SELECT id, name, nullable_text, nullable_int FROM pg_null_test WHERE name = ?", "allnulls")
		if err != nil {
			t.Fatalf("Get pointer null failed: %v", err)
		}
		if row.Text != nil {
			t.Errorf("expected nil Text pointer, got %v", *row.Text)
		}
		if row.Num != nil {
			t.Errorf("expected nil Num pointer, got %v", *row.Num)
		}
	})
}

// TestPGTypeConversionErrors tests data type conversion errors
func TestPGTypeConversionErrors(t *testing.T) {
	pgOnly(t)

	schema := Schema{
		Create: `
CREATE TABLE pg_type_test (
	id SERIAL PRIMARY KEY,
	val INTEGER NOT NULL
);`,
		Drop: `DROP TABLE IF EXISTS pg_type_test;`,
	}

	create, drop, _ := schema.Postgres()
	defer multiExec(pgdb, drop)
	multiExec(pgdb, create)

	_, _ = pgdb.Exec("INSERT INTO pg_type_test (val) VALUES (?)", 42)

	t.Run("WrongScanType", func(t *testing.T) {
		type WrongType struct {
			ID  int    `db:"id"`
			Val string `db:"val"` // integer → string can actually scan successfully
		}
		var wt WrongType
		err := pgdb.Get(&wt, "SELECT * FROM pg_type_test LIMIT 1")
		// PostgreSQL driver can usually scan int to string
		// Mainly verify it won't panic
		if err != nil {
			t.Logf("Scan int to string got error (driver dependent): %v", err)
		}
	})

	t.Run("InvalidSQL", func(t *testing.T) {
		_, err := pgdb.Exec("THIS IS NOT VALID SQL")
		if err == nil {
			t.Error("expected error from invalid SQL, got nil")
		}
	})

	t.Run("ConstraintViolation", func(t *testing.T) {
		// Insert violates NOT NULL constraint
		_, err := pgdb.Exec("INSERT INTO pg_type_test (val) VALUES (NULL)")
		if err == nil {
			t.Error("expected constraint violation error, got nil")
		}
	})
}

// TestPGConnectionTimeout tests connection timeout and retry
func TestPGConnectionTimeout(t *testing.T) {
	pgOnly(t)

	t.Run("ConnectContext_InvalidDSN", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()

		_, err := sqlex.ConnectContext(ctx, "postgres", "postgres://invalid:invalid@192.0.2.1:5432/noexist?sslmode=disable&connect_timeout=1")
		if err == nil {
			t.Error("expected error from invalid DSN, got nil")
		}
	})

	t.Run("QueryTimeout", func(t *testing.T) {
		// Use very short timeout to test query timeout
		ctx, cancel := context.WithTimeout(context.Background(), 1*time.Nanosecond)
		defer cancel()
		time.Sleep(1 * time.Millisecond) // ensure timeout

		var result int
		err := pgdb.GetContext(ctx, &result, "SELECT 1")
		if err == nil {
			t.Error("expected timeout error, got nil")
		}
	})
}

// TestPGConcurrency tests concurrent access
func TestPGConcurrency(t *testing.T) {
	pgOnly(t)

	create, drop, _ := pgSchema.Postgres()
	defer multiExec(pgdb, drop)
	multiExec(pgdb, create)

	t.Run("ConcurrentReads", func(t *testing.T) {
		// Insert some data first
		seedPGData(pgdb, t)

		var wg sync.WaitGroup
		errCount := int32(0)
		concurrency := 20

		for i := 0; i < concurrency; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				var users []IntUser
				err := pgdb.Select(&users, "SELECT * FROM pg_users ORDER BY id")
				if err != nil {
					atomic.AddInt32(&errCount, 1)
				}
				if len(users) < 3 {
					atomic.AddInt32(&errCount, 1)
				}
			}()
		}

		wg.Wait()
		if errCount > 0 {
			t.Errorf("concurrent reads: %d errors out of %d goroutines", errCount, concurrency)
		}
	})

	t.Run("ConcurrentWrites", func(t *testing.T) {
		var wg sync.WaitGroup
		errCount := int32(0)
		concurrency := 10

		for i := 0; i < concurrency; i++ {
			wg.Add(1)
			go func(idx int) {
				defer wg.Done()
				_, err := pgdb.Exec("INSERT INTO pg_users (name, email, age) VALUES (?, ?, ?)",
					fmt.Sprintf("Concurrent%d", idx),
					fmt.Sprintf("concurrent%d@test.com", idx),
					20+idx)
				if err != nil {
					atomic.AddInt32(&errCount, 1)
				}
			}(i)
		}

		wg.Wait()
		if errCount > 0 {
			t.Errorf("concurrent writes: %d errors out of %d goroutines", errCount, concurrency)
		}

		var count int
		pgdb.Get(&count, "SELECT COUNT(*) FROM pg_users WHERE name LIKE 'Concurrent%'")
		if count != concurrency {
			t.Errorf("expected %d concurrent rows, got %d", concurrency, count)
		}
	})

	t.Run("ConcurrentTx", func(t *testing.T) {
		var wg sync.WaitGroup
		errCount := int32(0)
		concurrency := 10

		for i := 0; i < concurrency; i++ {
			wg.Add(1)
			go func(idx int) {
				defer wg.Done()
				tx, err := pgdb.Beginx()
				if err != nil {
					atomic.AddInt32(&errCount, 1)
					return
				}
				_, err = tx.Exec("INSERT INTO pg_orders (user_id, amount, status) VALUES (?, ?, ?)",
					1, float64(idx)*10, "pending")
				if err != nil {
					tx.Rollback()
					atomic.AddInt32(&errCount, 1)
					return
				}
				err = tx.Commit()
				if err != nil {
					atomic.AddInt32(&errCount, 1)
				}
			}(i)
		}

		wg.Wait()
		if errCount > 0 {
			t.Errorf("concurrent tx: %d errors out of %d goroutines", errCount, concurrency)
		}
	})

	t.Run("ConcurrentHooks", func(t *testing.T) {
		db := sqlex.NewDb(pgdb.DB, pgdb.DriverName())
		var hookCallCount int32

		hook := &funcHook{
			before: func(ctx context.Context, event *sqlex.QueryEvent) context.Context {
				atomic.AddInt32(&hookCallCount, 1)
				return ctx
			},
			after: func(ctx context.Context, event *sqlex.QueryEvent) {
				atomic.AddInt32(&hookCallCount, 1)
			},
		}
		db.AddHook(hook)

		var wg sync.WaitGroup
		concurrency := 20

		for i := 0; i < concurrency; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				ctx := context.Background()
				var count int
				db.GetContext(ctx, &count, "SELECT COUNT(*) FROM pg_users")
			}()
		}

		wg.Wait()

		// 每次查询触发 before + after = 2 次
		expectedMin := int32(concurrency * 2)
		if hookCallCount < expectedMin {
			t.Errorf("expected at least %d hook calls, got %d", expectedMin, hookCallCount)
		}
	})
}

// ========================================================
// 6. NamedExt / BindExt 接口测试
// ========================================================

// TestPGNamedExtInterface NamedExt 统一接口在 PostgreSQL 下的测试
func TestPGNamedExtInterface(t *testing.T) {
	pgOnly(t)

	create, drop, _ := pgSchema.Postgres()
	defer multiExec(pgdb, drop)
	multiExec(pgdb, create)
	seedPGData(pgdb, t)

	getUserByName := func(ext sqlex.NamedExt, name string) (*IntUser, error) {
		var user IntUser
		err := ext.NamedGet(&user, `SELECT * FROM pg_users WHERE name = :name`,
			map[string]any{"name": name})
		if err != nil {
			return nil, err
		}
		return &user, nil
	}

	t.Run("DB", func(t *testing.T) {
		user, err := getUserByName(pgdb, "Alice")
		if err != nil {
			t.Fatalf("getUserByName via DB failed: %v", err)
		}
		if user.Email != "alice@example.com" {
			t.Errorf("expected alice@example.com, got %s", user.Email)
		}
	})

	t.Run("Tx", func(t *testing.T) {
		tx, err := pgdb.Beginx()
		if err != nil {
			t.Fatalf("Beginx failed: %v", err)
		}
		defer tx.Rollback()

		user, err := getUserByName(tx, "Bob")
		if err != nil {
			t.Fatalf("getUserByName via Tx failed: %v", err)
		}
		if user.Email != "bob@example.com" {
			t.Errorf("expected bob@example.com, got %s", user.Email)
		}
	})
}

// TestPGBindExtInterface BindExt 统一接口在 PostgreSQL 下的测试
func TestPGBindExtInterface(t *testing.T) {
	pgOnly(t)

	create, drop, _ := pgSchema.Postgres()
	defer multiExec(pgdb, drop)
	multiExec(pgdb, create)
	seedPGData(pgdb, t)

	listUsers := func(ext sqlex.BindExt, minAge int) ([]IntUser, error) {
		var users []IntUser
		err := ext.Select(&users, "SELECT * FROM pg_users WHERE age > ? ORDER BY age", minAge)
		return users, err
	}

	t.Run("DB", func(t *testing.T) {
		users, err := listUsers(pgdb, 26)
		if err != nil {
			t.Fatalf("listUsers via DB failed: %v", err)
		}
		if len(users) != 2 {
			t.Errorf("expected 2 users, got %d", len(users))
		}
	})

	t.Run("Tx", func(t *testing.T) {
		tx, err := pgdb.Beginx()
		if err != nil {
			t.Fatalf("Beginx failed: %v", err)
		}
		defer tx.Rollback()

		users, err := listUsers(tx, 26)
		if err != nil {
			t.Fatalf("listUsers via Tx failed: %v", err)
		}
		if len(users) != 2 {
			t.Errorf("expected 2 users, got %d", len(users))
		}
	})
}

// ========================================================
// 7. Rebind 在 PostgreSQL 中的正确性
// ========================================================

// TestPGRebind verifies Rebind correctly converts to $N placeholders in PostgreSQL
func TestPGRebind(t *testing.T) {
	pgOnly(t)

	create, drop, _ := pgSchema.Postgres()
	defer multiExec(pgdb, drop)
	multiExec(pgdb, create)
	seedPGData(pgdb, t)

	t.Run("BasicRebind", func(t *testing.T) {
		// 使用 ? 占位符，通过 Rebind 转换为 $N
		q := pgdb.Rebind("SELECT * FROM pg_users WHERE name = ? AND age > ?")
		expected := "SELECT * FROM pg_users WHERE name = $1 AND age > $2"
		if q != expected {
			t.Errorf("Rebind failed: expected %q, got %q", expected, q)
		}
	})

	t.Run("RebindQuery", func(t *testing.T) {
		// 现在可以直接用 ? 占位符，框架自动 Rebind
		var user IntUser
		err := pgdb.Get(&user, "SELECT * FROM pg_users WHERE name = ?", "Alice")
		if err != nil {
			t.Fatalf("Rebind query failed: %v", err)
		}
		if user.Name != "Alice" {
			t.Errorf("expected Alice, got %s", user.Name)
		}
	})

	t.Run("RebindEscapeQuestionMark", func(t *testing.T) {
		// ?? 转义为 ?（PostgreSQL JSONB 操作符）
		q := pgdb.Rebind("SELECT * FROM pg_users WHERE name = ? AND data ?? 'key'")
		expected := "SELECT * FROM pg_users WHERE name = $1 AND data ? 'key'"
		if q != expected {
			t.Errorf("Rebind escape failed: expected %q, got %q", expected, q)
		}
	})

	t.Run("AutoIN_WithRebind", func(t *testing.T) {
		// Select 使用 ? 占位符并自动 IN 展开
		var users []IntUser
		ids := []int{1, 3}
		err := pgdb.Select(&users, "SELECT * FROM pg_users WHERE id IN (?) ORDER BY id", ids)
		if err != nil {
			t.Fatalf("Auto-IN with Rebind failed: %v", err)
		}
		if len(users) != 2 {
			t.Fatalf("expected 2 users, got %d", len(users))
		}
		if users[0].Name != "Alice" || users[1].Name != "Charlie" {
			t.Errorf("unexpected users: %v, %v", users[0].Name, users[1].Name)
		}
	})
}
