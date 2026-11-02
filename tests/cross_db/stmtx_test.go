// stmtx_test.go — cross-database tests for Stmtx/TryStmtx nil safety and SQL propagation.
package cross_db_test

import (
	"context"
	"database/sql"
	"testing"

	sqlex "github.com/go-sqlex/sqlex"
)

// TestCrossDBStmtxNilSafety verifies that TryStmtx/TryStmtxContext return an
// error instead of panicking when given a nil statement (#13).
func TestCrossDBStmtxNilSafety(t *testing.T) {
	runWithSchema(crossSchema, t, func(db *sqlex.DB, t *testing.T, now string) {
		crossDBOnly(t)

		tx, err := db.Beginx()
		if err != nil {
			t.Fatalf("[%s] Beginx failed: %v", dbLabel(db), err)
		}
		defer tx.Rollback()

		cases := map[string]any{
			"typed_nil_sqlex_stmt": (*sqlex.Stmt)(nil),
			"typed_nil_sql_stmt":   (*sql.Stmt)(nil),
			"untyped_nil":          nil,
		}
		for name, stmt := range cases {
			t.Run(name, func(t *testing.T) {
				defer func() {
					if r := recover(); r != nil {
						t.Fatalf("[%s] TryStmtx(%s) panicked: %v", dbLabel(db), name, r)
					}
				}()
				if _, err := tx.TryStmtx(stmt); err == nil {
					t.Errorf("[%s] TryStmtx(%s) = nil error, want non-nil", dbLabel(db), name)
				}
				if _, err := tx.TryStmtxContext(context.Background(), stmt); err == nil {
					t.Errorf("[%s] TryStmtxContext(%s) = nil error, want non-nil", dbLabel(db), name)
				}
			})
		}
	})
}

// TestCrossDBStmtxQueryPropagation verifies that a statement moved into a
// transaction keeps its SQL so Hook events are not empty (#15).
func TestCrossDBStmtxQueryPropagation(t *testing.T) {
	runWithSchema(crossSchema, t, func(db *sqlex.DB, t *testing.T, now string) {
		crossDBOnly(t)

		const insertSQL = `INSERT INTO cross_users (name, email, age) VALUES (?, ?, ?)`
		stmt, err := db.Preparex(insertSQL)
		if err != nil {
			t.Fatalf("[%s] Preparex failed: %v", dbLabel(db), err)
		}
		defer stmt.Close()

		// Independent DB wrapper so the Hook does not leak into other tests.
		dbCopy := sqlex.NewDB(db.DB, db.DriverName())
		hook := &crossTestHook{}
		dbCopy.AddHook(hook)

		tx, err := dbCopy.Beginx()
		if err != nil {
			t.Fatalf("[%s] Beginx failed: %v", dbLabel(db), err)
		}
		defer tx.Rollback()

		if _, err = tx.Stmtx(stmt).ExecContext(context.Background(), "StmtxProp", "stmtx@example.com", 20); err != nil {
			t.Fatalf("[%s] ExecContext failed: %v", dbLabel(db), err)
		}

		// Preparex records the SQL after Rebind, so compare against the
		// driver-specific form ($1 / @p1) rather than the raw "?" query.
		wantSQL := db.Rebind(insertSQL)

		hook.mu.Lock()
		defer hook.mu.Unlock()
		for _, q := range hook.queries {
			if q == wantSQL {
				return
			}
		}
		t.Errorf("[%s] Hook did not receive the prepared SQL %q, queries=%v", dbLabel(db), wantSQL, hook.queries)
	})
}
