// e2e_test.go — end-to-end cross-database verification of sqlex new features
package cross_db_test

import (
	"context"
	"sync"
	"testing"

	sqlex "github.com/go-sqlex/sqlex"
)

// ========================================================
// TestCrossDBAutoINEndToEnd — verify Select/Get auto IN + autoRebind complete pipeline
// ========================================================
func TestCrossDBAutoINEndToEnd(t *testing.T) {
	runWithSchema(crossSchema, t, func(db *sqlex.DB, t *testing.T, now string) {
		crossDBOnly(t)
		seedCrossData(db, t)

		// Select auto IN + Rebind
		t.Run("Select", func(t *testing.T) {
			var users []CrossUser
			err := db.Select(&users,
				"SELECT * FROM cross_users WHERE age IN (?) ORDER BY age",
				[]int{25, 35})
			if err != nil {
				t.Fatalf("[%s] Select auto-IN end-to-end failed: %v", dbLabel(db), err)
			}
			if len(users) != 2 {
				t.Fatalf("[%s] expected 2 users, got %d", dbLabel(db), len(users))
			}
			if users[0].Name != "Bob" || users[1].Name != "Charlie" {
				t.Errorf("[%s] unexpected order: %s, %s", dbLabel(db), users[0].Name, users[1].Name)
			}
		})

		// Get auto IN (fetch one row)
		t.Run("Get_single", func(t *testing.T) {
			var count int
			err := db.Get(&count,
				"SELECT COUNT(*) FROM cross_users WHERE name IN (?)",
				[]string{"Alice", "Bob", "Charlie"})
			if err != nil {
				t.Fatalf("[%s] Get auto-IN count failed: %v", dbLabel(db), err)
			}
			if count != 3 {
				t.Errorf("[%s] expected count 3, got %d", dbLabel(db), count)
			}
		})
	})
}

// ========================================================
// TestCrossDBNamedGetAutoInRebind — verify NamedGet internal autoIn + autoRebind
// ========================================================
func TestCrossDBNamedGetAutoInRebind(t *testing.T) {
	runWithSchema(crossSchema, t, func(db *sqlex.DB, t *testing.T, now string) {
		crossDBOnly(t)
		seedCrossData(db, t)

		// NamedGet without IN (pure named parameter + autoRebind)
		var user CrossUser
		err := db.NamedGet(&user,
			`SELECT * FROM cross_users WHERE name = :name`,
			map[string]any{"name": "Alice"})
		if err != nil {
			t.Fatalf("[%s] NamedGet autoRebind failed: %v", dbLabel(db), err)
		}
		if user.Email != "alice@example.com" {
			t.Errorf("[%s] expected alice@example.com, got %s", dbLabel(db), user.Email)
		}

		// Specifically verify NamedGet works correctly on PostgreSQL (fix verification)
		if isPostgres(db) {
			t.Logf("[POSTGRES] NamedGet autoRebind works correctly (bug fix verified)")
		}
	})
}

// ========================================================
// TestCrossDBNamedSelectAutoInRebind — verify NamedSelect + slice parameter map
// ========================================================
func TestCrossDBNamedSelectAutoInRebind(t *testing.T) {
	runWithSchema(crossSchema, t, func(db *sqlex.DB, t *testing.T, now string) {
		crossDBOnly(t)
		seedCrossData(db, t)

		// NamedSelect + IN expansion + autoRebind
		var users []CrossUser
		err := db.NamedSelect(&users,
			`SELECT * FROM cross_users WHERE age IN (:ages) ORDER BY age`,
			map[string]any{"ages": []int{25, 35}})
		if err != nil {
			t.Fatalf("[%s] NamedSelect autoIn+Rebind failed: %v", dbLabel(db), err)
		}
		if len(users) != 2 {
			t.Fatalf("[%s] expected 2 users, got %d", dbLabel(db), len(users))
		}
		if users[0].Name != "Bob" || users[1].Name != "Charlie" {
			t.Errorf("[%s] unexpected: %s, %s", dbLabel(db), users[0].Name, users[1].Name)
		}

		if isPostgres(db) {
			t.Logf("[POSTGRES] NamedSelect autoIn+Rebind works correctly")
		}
	})
}

// ========================================================
// TestCrossDBTxNamedExecContextIN — verify tx.NamedExecContext + IN slice parameter
// ========================================================
func TestCrossDBTxNamedExecContextIN(t *testing.T) {
	runWithSchema(crossSchema, t, func(db *sqlex.DB, t *testing.T, now string) {
		crossDBOnly(t)
		seedCrossData(db, t)

		tx, err := db.Beginx()
		if err != nil {
			t.Fatalf("[%s] Beginx failed: %v", dbLabel(db), err)
		}
		defer tx.Rollback()

		ctx := context.Background()

		// tx.NamedExecContext + IN expansion
		result, err := tx.NamedExecContext(ctx,
			`DELETE FROM cross_orders WHERE user_id IN (:user_ids)`,
			map[string]any{"user_ids": []int{1, 2}})
		if err != nil {
			t.Fatalf("[%s] tx.NamedExecContext IN failed: %v", dbLabel(db), err)
		}
		rows, _ := result.RowsAffected()
		if rows != 3 {
			t.Errorf("[%s] expected 3 rows deleted, got %d", dbLabel(db), rows)
		}

		// Verify deletion took effect
		var count int
		tx.Get(&count, "SELECT COUNT(*) FROM cross_orders")
		if count != 0 {
			t.Errorf("[%s] expected 0 orders after delete, got %d", dbLabel(db), count)
		}
	})
}

// ========================================================
// TestCrossDBHookWithNamedQuery — verify Hook + Named query end-to-end
// ========================================================
func TestCrossDBHookWithNamedQuery(t *testing.T) {
	runWithSchema(crossSchema, t, func(db *sqlex.DB, t *testing.T, now string) {
		crossDBOnly(t)
		seedCrossData(db, t)

		dbCopy := sqlex.NewDb(db.DB, db.DriverName())
		hook := &crossTestHook{}
		dbCopy.AddHook(hook)

		ctx := context.Background()

		// Hook + NamedGet
		var user CrossUser
		err := dbCopy.NamedGetContext(ctx, &user,
			`SELECT * FROM cross_users WHERE name = :name`,
			map[string]any{"name": "Alice"})
		if err != nil {
			t.Fatalf("[%s] NamedGetContext with Hook failed: %v", dbLabel(db), err)
		}
		if user.Email != "alice@example.com" {
			t.Errorf("[%s] expected alice@example.com, got %s", dbLabel(db), user.Email)
		}

		// Hook + Tx + Named
		tx, err := dbCopy.BeginTxx(ctx, nil)
		if err != nil {
			t.Fatalf("[%s] BeginTxx failed: %v", dbLabel(db), err)
		}
		defer tx.Rollback()

		var users []CrossUser
		err = tx.NamedSelectContext(ctx, &users,
			`SELECT * FROM cross_users WHERE age > :min_age ORDER BY age`,
			map[string]any{"min_age": 0})
		if err != nil {
			t.Fatalf("[%s] Tx.NamedSelectContext with Hook failed: %v", dbLabel(db), err)
		}

		hook.mu.Lock()
		defer hook.mu.Unlock()

		if hook.beforeCount < 2 {
			t.Errorf("[%s] expected at least 2 BeforeQuery calls, got %d", dbLabel(db), hook.beforeCount)
		}
	})
}

// ========================================================
// TestCrossDBConnNamedExecAutoIn — verify Conn's NamedExecContext + autoIn + autoRebind
// ========================================================
func TestCrossDBConnNamedExecAutoIn(t *testing.T) {
	runWithSchema(crossSchema, t, func(db *sqlex.DB, t *testing.T, now string) {
		crossDBOnly(t)
		seedCrossData(db, t)

		ctx := context.Background()
		conn, err := db.Connx(ctx)
		if err != nil {
			t.Fatalf("[%s] Connx failed: %v", dbLabel(db), err)
		}
		defer conn.Close()

		// Conn.NamedExecContext + IN expansion
		result, err := conn.NamedExecContext(ctx,
			`DELETE FROM cross_orders WHERE user_id IN (:user_ids)`,
			map[string]any{"user_ids": []int{1, 2}})
		if err != nil {
			t.Fatalf("[%s] Conn.NamedExecContext IN failed: %v", dbLabel(db), err)
		}
		rows, _ := result.RowsAffected()
		if rows != 3 {
			t.Errorf("[%s] expected 3 rows deleted via Conn, got %d", dbLabel(db), rows)
		}
	})
}

// Suppress unused import warnings
var _ = sync.Mutex{}
