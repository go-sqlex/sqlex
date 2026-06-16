// strict_mode_test.go — Cross-DB integration tests for strict mode functionality
package cross_db_test

import (
	"context"
	"strings"
	"testing"

	sqlex "github.com/go-sqlex/sqlex"
)

// strictSchema defines the table structure used for strict mode tests
var strictSchema = Schema{
	Create: `
CREATE TABLE strict_test (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	name text,
	email text,
	age integer
);
`,
	Drop: `
DROP TABLE IF EXISTS strict_test;
`,
}

// StrictTestFull struct containing all columns
type StrictTestFull struct {
	ID    int    `db:"id"`
	Name  string `db:"name"`
	Email string `db:"email"`
	Age   int    `db:"age"`
}

// StrictTestPartial struct missing the age column
type StrictTestPartial struct {
	ID    int    `db:"id"`
	Name  string `db:"name"`
	Email string `db:"email"`
}

func loadStrictTestFixture(db *sqlex.DB, t *testing.T) {
	t.Helper()
	_, err := db.Exec(db.Rebind("INSERT INTO strict_test (name, email, age) VALUES (?, ?, ?)"), "Alice", "alice@example.com", 30)
	if err != nil {
		t.Fatal("insert fixture:", err)
	}
	_, err = db.Exec(db.Rebind("INSERT INTO strict_test (name, email, age) VALUES (?, ?, ?)"), "Bob", "bob@example.com", 25)
	if err != nil {
		t.Fatal("insert fixture:", err)
	}
}

// TestStrictModeDefaultValue verifies DB strict mode defaults to false (lenient mode)
func TestStrictModeDefaultValue(t *testing.T) {
	runWithSchema(strictSchema, t, func(db *sqlex.DB, t *testing.T, now string) {
		if db.IsStrict() {
			t.Error("DB.IsStrict() should default to false")
		}
	})
}

// TestStrictModeSetAndGet verifies SetStrict/IsStrict methods work correctly
func TestStrictModeSetAndGet(t *testing.T) {
	runWithSchema(strictSchema, t, func(db *sqlex.DB, t *testing.T, now string) {
		// Default false
		if db.IsStrict() {
			t.Error("DB.IsStrict() should default to false")
		}

		// Set to true
		db.SetStrict(true)
		defer db.SetStrict(false) // restore default
		if !db.IsStrict() {
			t.Error("DB.IsStrict() should be true after SetStrict(true)")
		}

		// Set back to false
		db.SetStrict(false)
		if db.IsStrict() {
			t.Error("DB.IsStrict() should be false after SetStrict(false)")
		}
	})
}

// TestLenientModeSelectDefault verifies Select silently ignores field mismatches in default lenient mode
func TestLenientModeSelectDefault(t *testing.T) {
	runWithSchema(strictSchema, t, func(db *sqlex.DB, t *testing.T, now string) {
		loadStrictTestFixture(db, t)

		// Default lenient mode: SELECT * to struct missing fields should succeed
		var results []StrictTestPartial
		err := db.Select(&results, "SELECT * FROM strict_test")
		if err != nil {
			t.Error("Select should succeed in default lenient mode when dest struct is missing columns:", err)
		}
		if len(results) != 2 {
			t.Errorf("Expected 2 results, got %d", len(results))
		}
		if len(results) > 0 && results[0].Name != "Alice" {
			t.Errorf("Expected first result name 'Alice', got '%s'", results[0].Name)
		}
	})
}

// TestLenientModeGetDefault verifies Get silently ignores field mismatches in default lenient mode
func TestLenientModeGetDefault(t *testing.T) {
	runWithSchema(strictSchema, t, func(db *sqlex.DB, t *testing.T, now string) {
		loadStrictTestFixture(db, t)

		// Default lenient mode: Get to struct missing fields should succeed
		var result StrictTestPartial
		err := db.Get(&result, selectTop1(db, "SELECT * FROM strict_test LIMIT 1"))
		if err != nil {
			t.Error("Get should succeed in default lenient mode when dest struct is missing columns:", err)
		}
		if result.Name != "Alice" {
			t.Errorf("Expected name 'Alice', got '%s'", result.Name)
		}
	})
}

// TestLenientModeStructScanDefault verifies StructScan silently ignores field mismatches in default lenient mode
func TestLenientModeStructScanDefault(t *testing.T) {
	runWithSchema(strictSchema, t, func(db *sqlex.DB, t *testing.T, now string) {
		loadStrictTestFixture(db, t)

		rows, err := db.Queryx(selectTop1(db, "SELECT * FROM strict_test LIMIT 1"))
		if err != nil {
			t.Fatal(err)
		}
		defer rows.Close()

		if rows.Next() {
			var result StrictTestPartial
			err = rows.StructScan(&result)
			if err != nil {
				t.Error("StructScan should succeed in default lenient mode:", err)
			}
			if result.Name != "Alice" {
				t.Errorf("Expected name 'Alice', got '%s'", result.Name)
			}
		}
	})
}

// TestStrictModeSelectError verifies Select reports error for field mismatches in strict mode
func TestStrictModeSelectError(t *testing.T) {
	runWithSchema(strictSchema, t, func(db *sqlex.DB, t *testing.T, now string) {
		loadStrictTestFixture(db, t)
		db.SetStrict(true)
		defer db.SetStrict(false) // restore default

		// Strict mode: SELECT * to struct missing fields should error
		var results []StrictTestPartial
		err := db.Select(&results, "SELECT * FROM strict_test")
		if err == nil {
			t.Error("Select should fail in strict mode when dest struct is missing columns")
		}
		if err != nil && !strings.Contains(err.Error(), "missing destination name") {
			t.Errorf("Error message should contain 'missing destination name', got: %v", err)
		}
		// Verify error message contains column name
		if err != nil && !strings.Contains(err.Error(), "age") {
			t.Errorf("Error message should contain missing column name 'age', got: %v", err)
		}
	})
}

// TestStrictModeGetError verifies Get reports error for field mismatches in strict mode
func TestStrictModeGetError(t *testing.T) {
	runWithSchema(strictSchema, t, func(db *sqlex.DB, t *testing.T, now string) {
		loadStrictTestFixture(db, t)
		db.SetStrict(true)
		defer db.SetStrict(false) // restore default

		// Strict mode: Get to struct missing fields should error
		var result StrictTestPartial
		err := db.Get(&result, selectTop1(db, "SELECT * FROM strict_test LIMIT 1"))
		if err == nil {
			t.Error("Get should fail in strict mode when dest struct is missing columns")
		}
		if err != nil && !strings.Contains(err.Error(), "missing destination name") {
			t.Errorf("Error message should contain 'missing destination name', got: %v", err)
		}
	})
}

// TestStrictModeStructScanError verifies StructScan reports error for field mismatches in strict mode
func TestStrictModeStructScanError(t *testing.T) {
	runWithSchema(strictSchema, t, func(db *sqlex.DB, t *testing.T, now string) {
		loadStrictTestFixture(db, t)
		db.SetStrict(true)
		defer db.SetStrict(false) // restore default

		// Strict mode: StructScan to struct missing fields should error
		rows, err := db.Queryx(selectTop1(db, "SELECT * FROM strict_test LIMIT 1"))
		if err != nil {
			t.Fatal(err)
		}
		defer rows.Close()

		if rows.Next() {
			var result StrictTestPartial
			err = rows.StructScan(&result)
			if err == nil {
				t.Error("StructScan should fail in strict mode when dest struct is missing columns")
			}
			if err != nil && !strings.Contains(err.Error(), "missing destination name") {
				t.Errorf("Error message should contain 'missing destination name', got: %v", err)
			}
		}
	})
}

// TestStrictModeFullStructNoError verifies fully matching struct doesn't error in strict mode
func TestStrictModeFullStructNoError(t *testing.T) {
	runWithSchema(strictSchema, t, func(db *sqlex.DB, t *testing.T, now string) {
		loadStrictTestFixture(db, t)
		db.SetStrict(true)
		defer db.SetStrict(false) // restore default

		// Strict mode: SELECT * to struct with all fields should succeed
		var results []StrictTestFull
		err := db.Select(&results, "SELECT * FROM strict_test")
		if err != nil {
			t.Error("Select should succeed in strict mode when dest struct has all columns:", err)
		}
		if len(results) != 2 {
			t.Errorf("Expected 2 results, got %d", len(results))
		}
	})
}

// TestStrictModeInheritanceTx verifies strict mode is correctly inherited from DB to Tx
func TestStrictModeInheritanceTx(t *testing.T) {
	runWithSchema(strictSchema, t, func(db *sqlex.DB, t *testing.T, now string) {
		loadStrictTestFixture(db, t)

		// Default lenient mode inherits to Tx
		tx, err := db.Beginx()
		if err != nil {
			t.Fatal(err)
		}
		defer tx.Rollback()

		if tx.IsStrict() {
			t.Error("Tx should inherit lenient mode from DB (default false)")
		}

		// Tx Select with missing fields should succeed (lenient mode)
		var results []StrictTestPartial
		err = tx.Select(&results, "SELECT * FROM strict_test")
		if err != nil {
			t.Error("Tx.Select should succeed in default lenient mode:", err)
		}

		tx.Rollback()

		// After setting DB to strict mode, new Tx should inherit
		db.SetStrict(true)
		defer db.SetStrict(false) // restore default
		tx2, err := db.Beginx()
		if err != nil {
			t.Fatal(err)
		}
		defer tx2.Rollback()

		if !tx2.IsStrict() {
			t.Error("Tx should inherit strict mode from DB")
		}

		// Tx Select with missing fields should error
		results = nil
		err = tx2.Select(&results, "SELECT * FROM strict_test")
		if err == nil {
			t.Error("Tx.Select should fail in strict mode when dest struct is missing columns")
		}
	})
}

// TestStrictModeInheritanceConn verifies strict mode is correctly inherited from DB to Conn
func TestStrictModeInheritanceConn(t *testing.T) {
	runWithSchema(strictSchema, t, func(db *sqlex.DB, t *testing.T, now string) {
		loadStrictTestFixture(db, t)
		ctx := context.Background()

		// Default lenient mode inherits to Conn
		conn, err := db.Connx(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer conn.Close()

		if conn.IsStrict() {
			t.Error("Conn should inherit lenient mode from DB (default false)")
		}

		// Conn SelectContext with missing fields should succeed (lenient mode)
		var results []StrictTestPartial
		err = conn.SelectContext(ctx, &results, "SELECT * FROM strict_test")
		if err != nil {
			t.Error("Conn.SelectContext should succeed in default lenient mode:", err)
		}

		conn.Close()

		// After setting DB to strict mode, new Conn should inherit
		db.SetStrict(true)
		defer db.SetStrict(false) // restore default
		conn2, err := db.Connx(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer conn2.Close()

		if !conn2.IsStrict() {
			t.Error("Conn should inherit strict mode from DB")
		}

		// Conn SelectContext with missing fields should error
		results = nil
		err = conn2.SelectContext(ctx, &results, "SELECT * FROM strict_test")
		if err == nil {
			t.Error("Conn.SelectContext should fail in strict mode when dest struct is missing columns")
		}
	})
}

// TestStrictModeTxOverride verifies Tx can independently override strict mode
func TestStrictModeTxOverride(t *testing.T) {
	runWithSchema(strictSchema, t, func(db *sqlex.DB, t *testing.T, now string) {
		loadStrictTestFixture(db, t)

		// DB default lenient mode, Tx overrides to strict mode
		tx, err := db.Beginx()
		if err != nil {
			t.Fatal(err)
		}
		defer tx.Rollback()

		// Tx overrides to strict mode
		tx.SetStrict(true)
		if !tx.IsStrict() {
			t.Error("Tx.IsStrict() should be true after SetStrict(true)")
		}

		// Tx Select with missing fields should error
		var results []StrictTestPartial
		err = tx.Select(&results, "SELECT * FROM strict_test")
		if err == nil {
			t.Error("Tx.Select should fail after overriding to strict mode")
		}
	})
}

// TestStrictModeNamedStmt verifies NamedStmt inherits strict mode behavior
func TestStrictModeNamedStmt(t *testing.T) {
	runWithSchema(strictSchema, t, func(db *sqlex.DB, t *testing.T, now string) {
		loadStrictTestFixture(db, t)

		// Default lenient mode: NamedStmt.Select with missing fields should succeed
		nstmt, err := db.PrepareNamed("SELECT * FROM strict_test WHERE name != :name")
		if err != nil {
			t.Fatal(err)
		}
		defer nstmt.Close()

		var results []StrictTestPartial
		err = nstmt.Select(&results, map[string]any{"name": "Nobody"})
		if err != nil {
			t.Error("NamedStmt.Select should succeed in default lenient mode:", err)
		}
		if len(results) != 2 {
			t.Errorf("Expected 2 results, got %d", len(results))
		}

		// Strict mode: NamedStmt.Select with missing fields should error
		db.SetStrict(true)
		defer db.SetStrict(false) // restore default
		nstmt2, err := db.PrepareNamed("SELECT * FROM strict_test WHERE name != :name")
		if err != nil {
			t.Fatal(err)
		}
		defer nstmt2.Close()

		results = nil
		err = nstmt2.Select(&results, map[string]any{"name": "Nobody"})
		if err == nil {
			t.Error("NamedStmt.Select should fail in strict mode when dest struct is missing columns")
		}
	})
}

// TestStrictModeNamedStmtGet verifies NamedStmt.Get inherits strict mode behavior
func TestStrictModeNamedStmtGet(t *testing.T) {
	runWithSchema(strictSchema, t, func(db *sqlex.DB, t *testing.T, now string) {
		loadStrictTestFixture(db, t)

		// Default lenient mode: NamedStmt.Get with missing fields should succeed
		nstmt, err := db.PrepareNamed(db.Rebind("SELECT * FROM strict_test WHERE name = :name"))
		if err != nil {
			t.Fatal(err)
		}
		defer nstmt.Close()

		var result StrictTestPartial
		err = nstmt.Get(&result, map[string]any{"name": "Alice"})
		if err != nil {
			t.Error("NamedStmt.Get should succeed in default lenient mode:", err)
		}
		if result.Name != "Alice" {
			t.Errorf("Expected name 'Alice', got '%s'", result.Name)
		}

		// Strict mode: NamedStmt.Get with missing fields should error
		db.SetStrict(true)
		defer db.SetStrict(false) // restore default
		nstmt2, err := db.PrepareNamed(db.Rebind("SELECT * FROM strict_test WHERE name = :name"))
		if err != nil {
			t.Fatal(err)
		}
		defer nstmt2.Close()

		err = nstmt2.Get(&result, map[string]any{"name": "Alice"})
		if err == nil {
			t.Error("NamedStmt.Get should fail in strict mode when dest struct is missing columns")
		}
	})
}

// TestStrictModePreparedStmt verifies Stmt/qStmt inherits strict mode behavior
func TestStrictModePreparedStmt(t *testing.T) {
	runWithSchema(strictSchema, t, func(db *sqlex.DB, t *testing.T, now string) {
		loadStrictTestFixture(db, t)

		// Default lenient mode: Stmt.Select with missing fields should succeed
		stmt, err := db.Preparex("SELECT * FROM strict_test")
		if err != nil {
			t.Fatal(err)
		}
		defer stmt.Close()

		var results []StrictTestPartial
		err = stmt.Select(&results)
		if err != nil {
			t.Error("Stmt.Select should succeed in default lenient mode:", err)
		}
		if len(results) != 2 {
			t.Errorf("Expected 2 results, got %d", len(results))
		}

		// Strict mode: Stmt.Select with missing fields should error
		db.SetStrict(true)
		defer db.SetStrict(false) // restore default
		stmt2, err := db.Preparex("SELECT * FROM strict_test")
		if err != nil {
			t.Fatal(err)
		}
		defer stmt2.Close()

		results = nil
		err = stmt2.Select(&results)
		if err == nil {
			t.Error("Stmt.Select should fail in strict mode when dest struct is missing columns")
		}
	})
}

// TestStrictModePreparedStmtGet verifies Stmt.Get inherits strict mode behavior
func TestStrictModePreparedStmtGet(t *testing.T) {
	runWithSchema(strictSchema, t, func(db *sqlex.DB, t *testing.T, now string) {
		loadStrictTestFixture(db, t)

		// Default lenient mode: Stmt.Get with missing fields should succeed
		stmt, err := db.Preparex(selectTop1(db, "SELECT * FROM strict_test LIMIT 1"))
		if err != nil {
			t.Fatal(err)
		}
		defer stmt.Close()

		var result StrictTestPartial
		err = stmt.Get(&result)
		if err != nil {
			t.Error("Stmt.Get should succeed in default lenient mode:", err)
		}
		if result.Name != "Alice" {
			t.Errorf("Expected name 'Alice', got '%s'", result.Name)
		}

		// Strict mode: Stmt.Get with missing fields should error
		db.SetStrict(true)
		defer db.SetStrict(false) // restore default
		stmt2, err := db.Preparex(selectTop1(db, "SELECT * FROM strict_test LIMIT 1"))
		if err != nil {
			t.Fatal(err)
		}
		defer stmt2.Close()

		err = stmt2.Get(&result)
		if err == nil {
			t.Error("Stmt.Get should fail in strict mode when dest struct is missing columns")
		}
	})
}

// TestStrictModeConnBeginTxx verifies strict mode propagates from Conn to Tx
func TestStrictModeConnBeginTxx(t *testing.T) {
	runWithSchema(strictSchema, t, func(db *sqlex.DB, t *testing.T, now string) {
		loadStrictTestFixture(db, t)
		ctx := context.Background()

		// DB strict mode → Conn inherits → Tx inherits
		db.SetStrict(true)
		defer db.SetStrict(false) // restore default
		conn, err := db.Connx(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer conn.Close()

		tx, err := conn.BeginTxx(ctx, nil)
		if err != nil {
			t.Fatal(err)
		}
		defer tx.Rollback()

		if !tx.IsStrict() {
			t.Error("Tx from Conn should inherit strict mode")
		}

		var results []StrictTestPartial
		err = tx.Select(&results, "SELECT * FROM strict_test")
		if err == nil {
			t.Error("Tx.Select should fail in strict mode inherited from Conn")
		}
	})
}

// TestStrictModeSelectContext verifies SelectContext behavior in strict mode
func TestStrictModeSelectContext(t *testing.T) {
	runWithSchema(strictSchema, t, func(db *sqlex.DB, t *testing.T, now string) {
		loadStrictTestFixture(db, t)
		ctx := context.Background()

		// Default lenient mode: SelectContext with missing fields should succeed
		var results []StrictTestPartial
		err := db.SelectContext(ctx, &results, "SELECT * FROM strict_test")
		if err != nil {
			t.Error("SelectContext should succeed in default lenient mode:", err)
		}
		if len(results) != 2 {
			t.Errorf("Expected 2 results, got %d", len(results))
		}

		// Strict mode: SelectContext with missing fields should error
		db.SetStrict(true)
		defer db.SetStrict(false) // restore default
		results = nil
		err = db.SelectContext(ctx, &results, "SELECT * FROM strict_test")
		if err == nil {
			t.Error("SelectContext should fail in strict mode when dest struct is missing columns")
		}
	})
}

// TestStrictModeGetContext verifies GetContext behavior in strict mode
func TestStrictModeGetContext(t *testing.T) {
	runWithSchema(strictSchema, t, func(db *sqlex.DB, t *testing.T, now string) {
		loadStrictTestFixture(db, t)
		ctx := context.Background()

		// Default lenient mode: GetContext with missing fields should succeed
		var result StrictTestPartial
		err := db.GetContext(ctx, &result, selectTop1(db, "SELECT * FROM strict_test LIMIT 1"))
		if err != nil {
			t.Error("GetContext should succeed in default lenient mode:", err)
		}
		if result.Name != "Alice" {
			t.Errorf("Expected name 'Alice', got '%s'", result.Name)
		}

		// Strict mode: GetContext with missing fields should error
		db.SetStrict(true)
		defer db.SetStrict(false) // restore default
		err = db.GetContext(ctx, &result, selectTop1(db, "SELECT * FROM strict_test LIMIT 1"))
		if err == nil {
			t.Error("GetContext should fail in strict mode when dest struct is missing columns")
		}
	})
}

// TestStrictModeErrorMessageContainsColumnInfo verifies error message contains column name and index
func TestStrictModeErrorMessageContainsColumnInfo(t *testing.T) {
	runWithSchema(strictSchema, t, func(db *sqlex.DB, t *testing.T, now string) {
		loadStrictTestFixture(db, t)
		db.SetStrict(true)
		defer db.SetStrict(false) // restore default

		var results []StrictTestPartial
		err := db.Select(&results, "SELECT * FROM strict_test")
		if err == nil {
			t.Fatal("Expected error in strict mode")
		}

		errMsg := err.Error()
		// Verify error message contains "missing destination name"
		if !strings.Contains(errMsg, "missing destination name") {
			t.Errorf("Error should contain 'missing destination name', got: %s", errMsg)
		}
		// Verify error message contains missing column name "age"
		if !strings.Contains(errMsg, "age") {
			t.Errorf("Error should contain missing column name 'age', got: %s", errMsg)
		}
		// Verify error message contains "index"
		if !strings.Contains(errMsg, "index") {
			t.Errorf("Error should contain 'index' for column position, got: %s", errMsg)
		}
	})
}

// TestStrictModeNamedStmtInTx verifies NamedStmt in transaction inherits strict mode
func TestStrictModeNamedStmtInTx(t *testing.T) {
	runWithSchema(strictSchema, t, func(db *sqlex.DB, t *testing.T, now string) {
		loadStrictTestFixture(db, t)

		// Create NamedStmt on DB (default lenient mode)
		nstmt, err := db.PrepareNamed("SELECT * FROM strict_test WHERE name = :name")
		if err != nil {
			t.Fatal(err)
		}
		defer nstmt.Close()

		// Using NamedStmt in default lenient Tx should succeed
		tx, err := db.Beginx()
		if err != nil {
			t.Fatal(err)
		}
		defer tx.Rollback()

		txNstmt := tx.NamedStmt(nstmt)
		var result StrictTestPartial
		err = txNstmt.Get(&result, map[string]any{"name": "Alice"})
		if err != nil {
			t.Error("NamedStmt in default lenient Tx should succeed:", err)
		}
	})
}
