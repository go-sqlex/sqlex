// rebind_test.go — Task 3: Cross-DB integration tests for Rebind SQL rewriting
//
// Requirement coverage: 2.1–2.9, 12.2–12.5, 12.14
package cross_db_test

import (
	"database/sql"
	"errors"
	"testing"

	sqlex "github.com/go-sqlex/sqlex"
)

// ========================================================
// TestCrossDBRebindAutomatic — verify automatic Rebind (? → $N on PG, ? unchanged on MySQL)
// ========================================================
func TestCrossDBRebindAutomatic(t *testing.T) {
	runWithSchema(crossSchema, t, func(db *sqlex.DB, t *testing.T, now string) {
		crossDBOnly(t)
		seedCrossData(db, t)

		// db.Get auto Rebind
		var user CrossUser
		err := db.Get(&user, "SELECT * FROM cross_users WHERE name = ?", "Alice")
		if err != nil {
			t.Fatalf("[%s] Get with ? placeholder failed: %v", dbLabel(db), err)
		}
		if user.Email != "alice@example.com" {
			t.Errorf("[%s] expected alice@example.com, got %s", dbLabel(db), user.Email)
		}

		// db.Select auto Rebind
		var users []CrossUser
		err = db.Select(&users, "SELECT * FROM cross_users WHERE age > ? ORDER BY age", 26)
		if err != nil {
			t.Fatalf("[%s] Select with ? placeholder failed: %v", dbLabel(db), err)
		}
		if len(users) != 2 {
			t.Errorf("[%s] expected 2 users, got %d", dbLabel(db), len(users))
		}

		// db.Exec auto Rebind
		result, err := db.Exec("UPDATE cross_users SET age = ? WHERE name = ?", 31, "Alice")
		if err != nil {
			t.Fatalf("[%s] Exec with ? placeholder failed: %v", dbLabel(db), err)
		}
		rows, _ := result.RowsAffected()
		if rows != 1 {
			t.Errorf("[%s] expected 1 row affected, got %d", dbLabel(db), rows)
		}
	})
}

// ========================================================
// TestCrossDBRebindMultipleParams — verify correct numbering of multiple ? parameters
// ========================================================
func TestCrossDBRebindMultipleParams(t *testing.T) {
	runWithSchema(crossSchema, t, func(db *sqlex.DB, t *testing.T, now string) {
		crossDBOnly(t)
		seedCrossData(db, t)

		var user CrossUser
		err := db.Get(&user,
			"SELECT * FROM cross_users WHERE name = ? AND age = ? AND email = ?",
			"Alice", 30, "alice@example.com")
		if err != nil {
			t.Fatalf("[%s] Get with 3 params failed: %v", dbLabel(db), err)
		}
		if user.Name != "Alice" {
			t.Errorf("[%s] expected Alice, got %s", dbLabel(db), user.Name)
		}
	})
}

// ========================================================
// TestCrossDBRebindEscape — verify ?? and \? escape to literal ?
// ========================================================
func TestCrossDBRebindEscape(t *testing.T) {
	runWithSchema(crossSchema, t, func(db *sqlex.DB, t *testing.T, now string) {
		crossDBOnly(t)

		// ?? escapes to literal ?
		result := db.Rebind("SELECT * FROM cross_users WHERE name = ? AND data ?? 'key'")
		if isPostgres(db) {
			expected := "SELECT * FROM cross_users WHERE name = $1 AND data ? 'key'"
			if result != expected {
				t.Errorf("[POSTGRES] Rebind ?? escape: expected %q, got %q", expected, result)
			}
		}

		// \? escapes to literal ?
		result = db.Rebind(`SELECT * FROM cross_users WHERE name = ? AND x \? y`)
		if isPostgres(db) {
			expected := `SELECT * FROM cross_users WHERE name = $1 AND x ? y`
			if result != expected {
				t.Errorf("[POSTGRES] Rebind \\? escape: expected %q, got %q", expected, result)
			}
		}
	})
}

// ========================================================
// TestCrossDBRebindStringLiteral — verify ? inside string literals is not replaced
// ========================================================
func TestCrossDBRebindStringLiteral(t *testing.T) {
	runWithSchema(crossSchema, t, func(db *sqlex.DB, t *testing.T, now string) {
		crossDBOnly(t)

		// ? inside single-quoted string literal should not be replaced
		result := db.Rebind("SELECT ? WHERE name = 'What?'")
		if isPostgres(db) {
			expected := "SELECT $1 WHERE name = 'What?'"
			if result != expected {
				t.Errorf("[POSTGRES] Rebind string literal: expected %q, got %q", expected, result)
			}
		}
		if isMySQL(db) {
			expected := "SELECT ? WHERE name = 'What?'"
			if result != expected {
				t.Errorf("[MYSQL] Rebind string literal: expected %q, got %q", expected, result)
			}
		}

		// String literal with '' escaped quotes
		result = db.Rebind("SELECT ? WHERE name = 'it''s ?'")
		if isPostgres(db) {
			expected := "SELECT $1 WHERE name = 'it''s ?'"
			if result != expected {
				t.Errorf("[POSTGRES] Rebind escaped quote: expected %q, got %q", expected, result)
			}
		}
	})
}

// ========================================================
// TestCrossDBRebindInTx — verify queries in transaction use the same bind type as DB
// ========================================================
func TestCrossDBRebindInTx(t *testing.T) {
	runWithSchema(crossSchema, t, func(db *sqlex.DB, t *testing.T, now string) {
		crossDBOnly(t)
		seedCrossData(db, t)

		tx, err := db.Beginx()
		if err != nil {
			t.Fatalf("[%s] Beginx failed: %v", dbLabel(db), err)
		}
		defer tx.Rollback()

		// Use ? placeholder in transaction, should auto Rebind
		var user CrossUser
		err = tx.Get(&user, "SELECT * FROM cross_users WHERE name = ?", "Alice")
		if err != nil {
			t.Fatalf("[%s] Tx.Get with ? failed: %v", dbLabel(db), err)
		}
		if user.Email != "alice@example.com" {
			t.Errorf("[%s] expected alice@example.com, got %s", dbLabel(db), user.Email)
		}

		// Verify Tx.Rebind returns correct format
		q := tx.Rebind("SELECT * FROM cross_users WHERE name = ?")
		if isPostgres(db) && q != "SELECT * FROM cross_users WHERE name = $1" {
			t.Errorf("[POSTGRES] Tx.Rebind failed: %q", q)
		}

		var users []CrossUser
		err = tx.Select(&users, "SELECT * FROM cross_users WHERE age > ? ORDER BY age", 0)
		if err != nil {
			t.Fatalf("[%s] Tx.Select failed: %v", dbLabel(db), err)
		}
		if len(users) != 3 {
			t.Errorf("[%s] expected 3 users in tx, got %d", dbLabel(db), len(users))
		}
	})
}

// Suppress unused import warnings
var (
	_ = sql.ErrNoRows
	_ = errors.New
)
