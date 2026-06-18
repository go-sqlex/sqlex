// edge_test.go — edge case and error scenario tests
package cross_db_test

import (
	"context"
	"database/sql"
	"testing"

	sqlex "github.com/go-sqlex/sqlex"
)

// ========================================================
// TestCrossDBNilDestination — verify Get/Select returns error when destination is nil
// ========================================================
func TestCrossDBNilDestination(t *testing.T) {
	runWithSchema(crossSchema, t, func(db *sqlex.DB, t *testing.T, now string) {
		crossDBOnly(t)
		seedCrossData(db, t)

		// Get with nil destination — should return error, not panic
		err := db.Get(nil, selectTop1(db, "SELECT * FROM cross_users LIMIT 1"))
		if err == nil {
			t.Errorf("[%s] Get(nil) should return error", dbLabel(db))
		}
	})
}

// ========================================================
// TestCrossDBInvalidSelectDest — verify Select returns error when destination is not a slice pointer
// ========================================================
func TestCrossDBInvalidSelectDest(t *testing.T) {
	runWithSchema(crossSchema, t, func(db *sqlex.DB, t *testing.T, now string) {
		crossDBOnly(t)
		seedCrossData(db, t)

		// Non-pointer destination
		var user CrossUser
		err := db.Select(user, "SELECT * FROM cross_users")
		if err == nil {
			t.Errorf("[%s] Select(non-pointer) should return error", dbLabel(db))
		}

		// Non-slice pointer destination
		err = db.Select(&user, "SELECT * FROM cross_users")
		if err == nil {
			t.Errorf("[%s] Select(&struct) should return error", dbLabel(db))
		}
	})
}

// ========================================================
// TestCrossDBNullTypes — verify sql.NullString/sql.NullInt64 handles NULL
// ========================================================
func TestCrossDBNullTypes(t *testing.T) {
	var nullSchema = Schema{
		Create: `
CREATE TABLE cross_null_test (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	name TEXT,
	age INTEGER
);
`,
		Drop: `
DROP TABLE IF EXISTS cross_null_test;
`,
	}

	runWithSchema(nullSchema, t, func(db *sqlex.DB, t *testing.T, now string) {
		crossDBOnly(t)

		// Insert NULL values
		db.MustExec("INSERT INTO cross_null_test (name, age) VALUES (NULL, NULL)")
		db.MustExec("INSERT INTO cross_null_test (name, age) VALUES ('HasName', 25)")

		type NullRow struct {
			ID   int            `db:"id"`
			Name sql.NullString `db:"name"`
			Age  sql.NullInt64  `db:"age"`
		}

		// Read NULL row
		var nullRow NullRow
		err := db.Get(&nullRow, "SELECT * FROM cross_null_test WHERE id = 1")
		if err != nil {
			t.Fatalf("[%s] Get NULL row failed: %v", dbLabel(db), err)
		}
		if nullRow.Name.Valid {
			t.Errorf("[%s] expected Name.Valid = false for NULL", dbLabel(db))
		}
		if nullRow.Age.Valid {
			t.Errorf("[%s] expected Age.Valid = false for NULL", dbLabel(db))
		}

		// Read non-NULL row
		var validRow NullRow
		err = db.Get(&validRow, "SELECT * FROM cross_null_test WHERE id = 2")
		if err != nil {
			t.Fatalf("[%s] Get valid row failed: %v", dbLabel(db), err)
		}
		if !validRow.Name.Valid || validRow.Name.String != "HasName" {
			t.Errorf("[%s] expected Name='HasName', got %+v", dbLabel(db), validRow.Name)
		}
		if !validRow.Age.Valid || validRow.Age.Int64 != 25 {
			t.Errorf("[%s] expected Age=25, got %+v", dbLabel(db), validRow.Age)
		}
	})
}

// ========================================================
// TestCrossDBNestedStruct — verify nested struct column mapping
// ========================================================
func TestCrossDBNestedStruct(t *testing.T) {
	runWithSchema(crossSchema, t, func(db *sqlex.DB, t *testing.T, now string) {
		crossDBOnly(t)
		seedCrossData(db, t)

		// Nested struct — all fields flattened for mapping
		type NameInfo struct {
			Name  string `db:"name"`
			Email string `db:"email"`
		}
		type UserWithNested struct {
			ID       int `db:"id"`
			NameInfo     // embedded struct
			Age      int `db:"age"`
		}

		var user UserWithNested
		err := db.Get(&user, "SELECT id, name, email, age FROM cross_users WHERE name = ?", "Alice")
		if err != nil {
			t.Fatalf("[%s] Get with nested struct failed: %v", dbLabel(db), err)
		}
		if user.Name != "Alice" || user.Email != "alice@example.com" {
			t.Errorf("[%s] unexpected nested: name=%s, email=%s", dbLabel(db), user.Name, user.Email)
		}
	})
}

// ========================================================
// TestCrossDBConnectInvalidDSN — verify invalid DSN returns error without panic
// ========================================================
func TestCrossDBConnectInvalidDSN(t *testing.T) {
	crossDBOnly(t)

	// Connect with invalid MySQL DSN
	_, err := sqlex.Connect("mysql", "invalid:dsn@tcp(127.0.0.1:0)/nonexist")
	if err == nil {
		t.Error("Connect with invalid MySQL DSN should return error")
	}

	// ConnectContext with invalid PostgreSQL DSN
	ctx := context.Background()
	_, err = sqlex.ConnectContext(ctx, "postgres", "host=invalid_host_that_does_not_exist port=0 dbname=nonexist sslmode=disable connect_timeout=1")
	if err == nil {
		t.Error("ConnectContext with invalid PG DSN should return error")
	}
}
