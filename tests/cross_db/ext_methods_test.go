// ext_methods_test.go — non-Context methods tested across DB/Tx/Conn via factory pattern
package cross_db_test

import (
	"context"
	"database/sql"
	"testing"

	sqlex "github.com/go-sqlex/sqlex"
	"github.com/go-sqlex/sqlex/reflectx"
)

// testExt is a composite interface for testing — DB, Tx, Conn all implement it.
// Combines BindExt + NamedExt + Ext metadata + MustExec + strict/mapper accessors.
type testExt interface {
	sqlex.BindExt  // Select, Get, Exec, Queryx, QueryRowx (+ Context)
	sqlex.NamedExt // NamedExec, NamedGet, NamedSelect, NamedQuery (+ Context)
	sqlex.Ext      // DriverName, Rebind, BindNamed

	MustExec(query string, args ...any) sql.Result
	GetMapper() *reflectx.Mapper
	SetStrict(bool)
	IsStrict() bool
}

// extFactory creates a testExt instance and returns a cleanup function.
type extFactory struct {
	name    string
	factory func(t *testing.T) (testExt, func())
}

// extFactories returns all DB/Tx/Conn implementations to test.
func extFactories(db *sqlex.DB) []extFactory {
	ctx := context.Background()
	return []extFactory{
		{"DB", func(t *testing.T) (testExt, func()) {
			return db, func() {}
		}},
		{"Tx", func(t *testing.T) (testExt, func()) {
			tx, err := db.Beginx()
			if err != nil {
				t.Fatalf("Beginx: %v", err)
			}
			return tx, func() { _ = tx.Rollback() }
		}},
		{"Conn", func(t *testing.T) (testExt, func()) {
			conn, err := db.Connx(ctx)
			if err != nil {
				t.Fatalf("Connx: %v", err)
			}
			return conn, func() { _ = conn.Close() }
		}},
	}
}

// reseed clears and re-inserts test data for a fresh start.
func reseed(db *sqlex.DB, t *testing.T) {
	t.Helper()
	db.MustExec("DELETE FROM cross_users")
	db.MustExec("DELETE FROM cross_orders")
	seedCrossData(db, t)
}

// ========================================================
// TestExtNonContextCRUD — Select/Get/Queryx/QueryRowx/Exec/MustExec
// ========================================================
func TestExtNonContextCRUD(t *testing.T) {
	runWithSchema(crossSchema, t, func(db *sqlex.DB, t *testing.T, now string) {
		for _, f := range extFactories(db) {
			t.Run(f.name, func(t *testing.T) {
				reseed(db, t)
				ext, cleanup := f.factory(t)
				defer cleanup()

				// Select
				var users []CrossUser
				if err := ext.Select(&users, "SELECT * FROM cross_users ORDER BY id"); err != nil {
					t.Fatalf("Select: %v", err)
				}
				if len(users) != 3 {
					t.Errorf("expected 3, got %d", len(users))
				}

				// Get
				var user CrossUser
				if err := ext.Get(&user, "SELECT * FROM cross_users WHERE name = ?", "Alice"); err != nil {
					t.Fatalf("Get: %v", err)
				}

				// Queryx
				rows, err := ext.Queryx("SELECT name FROM cross_users ORDER BY id")
				if err != nil {
					t.Fatalf("Queryx: %v", err)
				}
				count := 0
				for rows.Next() {
					var name string
					rows.Scan(&name)
					count++
				}
				rows.Close()
				if count != 3 {
					t.Errorf("expected 3 rows, got %d", count)
				}

				// QueryRowx + StructScan
				var u2 CrossUser
				if err := ext.QueryRowx("SELECT * FROM cross_users WHERE name = ?", "Bob").StructScan(&u2); err != nil {
					t.Fatalf("QueryRowx.StructScan: %v", err)
				}
				if u2.Age != 25 {
					t.Errorf("expected age 25, got %d", u2.Age)
				}

				// Exec
				res, err := ext.Exec("UPDATE cross_users SET age = ? WHERE name = ?", 32, "Alice")
				if err != nil {
					t.Fatalf("Exec: %v", err)
				}
				n, _ := res.RowsAffected()
				if n != 1 {
					t.Errorf("expected 1 affected, got %d", n)
				}

				// MustExec
				ext.MustExec("UPDATE cross_users SET age = ? WHERE name = ?", 33, "Alice")
			})
		}
	})
}

// ========================================================
// TestExtNamedNonContext — NamedExec/NamedGet/NamedSelect/NamedQuery
// ========================================================
func TestExtNamedNonContext(t *testing.T) {
	runWithSchema(crossSchema, t, func(db *sqlex.DB, t *testing.T, now string) {
		for _, f := range extFactories(db) {
			t.Run(f.name, func(t *testing.T) {
				reseed(db, t)
				ext, cleanup := f.factory(t)
				defer cleanup()

				// NamedExec
				_, err := ext.NamedExec(
					"INSERT INTO cross_users (name, email, age) VALUES (:name, :email, :age)",
					map[string]any{"name": "NamedNC", "email": "nc@test.com", "age": 40})
				if err != nil {
					t.Fatalf("NamedExec: %v", err)
				}

				// NamedGet
				var user CrossUser
				if err := ext.NamedGet(&user,
					"SELECT * FROM cross_users WHERE name = :name",
					map[string]any{"name": "NamedNC"}); err != nil {
					t.Fatalf("NamedGet: %v", err)
				}
				if user.Age != 40 {
					t.Errorf("expected age 40, got %d", user.Age)
				}

				// NamedSelect
				var users []CrossUser
				if err := ext.NamedSelect(&users,
					"SELECT * FROM cross_users WHERE age > :min ORDER BY age",
					map[string]any{"min": 28}); err != nil {
					t.Fatalf("NamedSelect: %v", err)
				}
				if len(users) < 1 {
					t.Error("expected at least 1 user")
				}

				// NamedQuery
				nrows, err := ext.NamedQuery(
					"SELECT name FROM cross_users WHERE name = :name",
					map[string]any{"name": "Alice"})
				if err != nil {
					t.Fatalf("NamedQuery: %v", err)
				}
				nrows.Close()
			})
		}
	})
}

// ========================================================
// TestExtMetaAndRebind — DriverName/GetMapper/SetStrict/IsStrict/Rebind/BindNamed
// ========================================================
func TestExtMetaAndRebind(t *testing.T) {
	runWithSchema(crossSchema, t, func(db *sqlex.DB, t *testing.T, now string) {
		for _, f := range extFactories(db) {
			t.Run(f.name, func(t *testing.T) {
				ext, cleanup := f.factory(t)
				defer cleanup()

				// DriverName
				if ext.DriverName() == "" {
					t.Error("DriverName should not be empty")
				}

				// GetMapper
				if ext.GetMapper() == nil {
					t.Error("GetMapper should not be nil")
				}

				// SetStrict / IsStrict
				ext.SetStrict(true)
				if !ext.IsStrict() {
					t.Error("IsStrict should be true")
				}
				ext.SetStrict(false)
				if ext.IsStrict() {
					t.Error("IsStrict should be false")
				}

				// Rebind
				if ext.Rebind("SELECT * FROM t WHERE id = ?") == "" {
					t.Error("Rebind should not return empty")
				}

				// BindNamed
				q, args, err := ext.BindNamed("SELECT * FROM t WHERE name = :name", map[string]any{"name": "x"})
				if err != nil {
					t.Fatalf("BindNamed: %v", err)
				}
				if q == "" || len(args) != 1 {
					t.Errorf("BindNamed result invalid: q=%q args=%v", q, args)
				}
			})
		}
	})
}

// ========================================================
// TestTxStmtMethods — Stmtx/TryStmtx/NamedStmt (Tx-only)
// ========================================================
func TestTxStmtMethods(t *testing.T) {
	runWithSchema(crossSchema, t, func(db *sqlex.DB, t *testing.T, now string) {
		seedCrossData(db, t)

		tx, err := db.Beginx()
		if err != nil {
			t.Fatalf("Beginx: %v", err)
		}
		defer tx.Rollback()

		stmt, err := tx.Preparex("SELECT name FROM cross_users WHERE age > ?")
		if err != nil {
			t.Fatalf("Preparex: %v", err)
		}
		defer stmt.Close()

		// Stmtx
		s2 := tx.Stmtx(stmt)
		defer s2.Close()

		// TryStmtx — valid
		s3, err := tx.TryStmtx(stmt)
		if err != nil {
			t.Fatalf("TryStmtx: %v", err)
		}
		defer s3.Close()

		// TryStmtx — invalid type
		if _, err := tx.TryStmtx("not a stmt"); err == nil {
			t.Error("TryStmtx should error on invalid type")
		}

		// StmtxContext / TryStmtxContext
		ctx := context.Background()
		s4 := tx.StmtxContext(ctx, stmt)
		defer s4.Close()

		s5, err := tx.TryStmtxContext(ctx, stmt)
		if err != nil {
			t.Fatalf("TryStmtxContext: %v", err)
		}
		defer s5.Close()

		// PrepareNamed + NamedStmt
		nstmt, err := tx.PrepareNamed("SELECT name FROM cross_users WHERE name = :name")
		if err != nil {
			t.Fatalf("PrepareNamed: %v", err)
		}
		defer nstmt.Close()

		ns2 := tx.NamedStmt(nstmt)
		defer ns2.Close()

		ns3 := tx.NamedStmtContext(ctx, nstmt)
		defer ns3.Close()
	})
}

// ========================================================
// TestRowMethods — Columns/ColumnTypes/Err/GetMapper/SliceScan/MapScan
// ========================================================
func TestRowMethods(t *testing.T) {
	runWithSchema(crossSchema, t, func(db *sqlex.DB, t *testing.T, now string) {
		seedCrossData(db, t)

		t.Run("SliceScan", func(t *testing.T) {
			row := db.QueryRowx("SELECT name, email, age FROM cross_users WHERE name = ?", "Alice")
			vals, err := row.SliceScan()
			if err != nil {
				t.Fatalf("SliceScan: %v", err)
			}
			if len(vals) != 3 {
				t.Errorf("expected 3 values, got %d", len(vals))
			}
		})

		t.Run("MapScan", func(t *testing.T) {
			row := db.QueryRowx("SELECT name, email, age FROM cross_users WHERE name = ?", "Bob")
			m := map[string]any{}
			if err := row.MapScan(m); err != nil {
				t.Fatalf("MapScan: %v", err)
			}
			if len(m) < 3 {
				t.Errorf("expected at least 3 keys, got %d", len(m))
			}
		})

		t.Run("Columns_ColumnTypes", func(t *testing.T) {
			row := db.QueryRowx("SELECT name, email FROM cross_users WHERE name = ?", "Alice")
			cols, err := row.Columns()
			if err != nil {
				t.Fatalf("Columns: %v", err)
			}
			if len(cols) != 2 {
				t.Errorf("expected 2 columns, got %d", len(cols))
			}
			ctypes, err := row.ColumnTypes()
			if err != nil {
				t.Fatalf("ColumnTypes: %v", err)
			}
			if len(ctypes) != 2 {
				t.Errorf("expected 2 types, got %d", len(ctypes))
			}
		})

		t.Run("Err_GetMapper", func(t *testing.T) {
			row := db.QueryRowx("SELECT name FROM cross_users WHERE name = ?", "Alice")
			if row.GetMapper() == nil {
				t.Error("GetMapper should not be nil")
			}
			row2 := db.QueryRowx("SELECT name FROM cross_users WHERE name = ?", "NonExistent")
			var name string
			_ = row2.Scan(&name)
		})
	})
}

// ========================================================
// TestConnSpecific — PreparexContext/PrepareNamedContext/BeginTxx (Conn-only)
// ========================================================
func TestConnSpecific(t *testing.T) {
	runWithSchema(crossSchema, t, func(db *sqlex.DB, t *testing.T, now string) {
		seedCrossData(db, t)

		ctx := context.Background()
		conn, err := db.Connx(ctx)
		if err != nil {
			t.Fatalf("Connx: %v", err)
		}
		defer conn.Close()

		// PreparexContext
		stmt, err := conn.PreparexContext(ctx, "SELECT name FROM cross_users WHERE age > ?")
		if err != nil {
			t.Fatalf("PreparexContext: %v", err)
		}
		defer stmt.Close()
		var name string
		if err := stmt.GetContext(ctx, &name, 20); err != nil {
			t.Fatalf("stmt.GetContext: %v", err)
		}

		// PrepareNamedContext
		nstmt, err := conn.PrepareNamedContext(ctx, "SELECT name FROM cross_users WHERE name = :name")
		if err != nil {
			t.Fatalf("PrepareNamedContext: %v", err)
		}
		defer nstmt.Close()

		// BeginTxx from Conn
		tx, err := conn.BeginTxx(ctx, nil)
		if err != nil {
			t.Fatalf("BeginTxx: %v", err)
		}
		tx.MustExec("INSERT INTO cross_users (name, email, age) VALUES ('ConnTx', 'ctx@test.com', 50)")
		tx.Commit()
	})
}
