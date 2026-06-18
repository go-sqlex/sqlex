// query_test.go — cross-database integration tests for core query APIs
package cross_db_test

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	sqlex "github.com/go-sqlex/sqlex"
)

// ========================================================
// TestCrossDBBasicCRUD — verify Get/Select/Exec basic CRUD
// ========================================================
func TestCrossDBBasicCRUD(t *testing.T) {
	runWithSchema(crossSchema, t, func(db *sqlex.DB, t *testing.T, now string) {
		crossDBOnly(t)
		seedCrossData(db, t)

		// Select multiple rows
		var users []CrossUser
		err := db.Select(&users, "SELECT * FROM cross_users ORDER BY id")
		if err != nil {
			t.Fatalf("[%s] Select failed: %v", dbLabel(db), err)
		}
		if len(users) != 3 {
			t.Fatalf("[%s] expected 3 users, got %d", dbLabel(db), len(users))
		}
		if users[0].Name != "Alice" || users[1].Name != "Bob" || users[2].Name != "Charlie" {
			t.Errorf("[%s] unexpected user names: %v", dbLabel(db), users)
		}

		// Get single row
		var user CrossUser
		err = db.Get(&user, "SELECT * FROM cross_users WHERE name = ?", "Alice")
		if err != nil {
			t.Fatalf("[%s] Get failed: %v", dbLabel(db), err)
		}
		if user.Email != "alice@example.com" || user.Age != 30 {
			t.Errorf("[%s] unexpected user: %+v", dbLabel(db), user)
		}

		// Exec UPDATE
		result, err := db.Exec("UPDATE cross_users SET age = ? WHERE name = ?", 31, "Alice")
		if err != nil {
			t.Fatalf("[%s] Exec UPDATE failed: %v", dbLabel(db), err)
		}
		rows, _ := result.RowsAffected()
		if rows != 1 {
			t.Errorf("[%s] expected 1 row affected, got %d", dbLabel(db), rows)
		}

		// Exec DELETE
		result, err = db.Exec("DELETE FROM cross_orders WHERE status = ?", "pending")
		if err != nil {
			t.Fatalf("[%s] Exec DELETE failed: %v", dbLabel(db), err)
		}
		rows, _ = result.RowsAffected()
		if rows != 1 {
			t.Errorf("[%s] expected 1 row deleted, got %d", dbLabel(db), rows)
		}
	})
}

// ========================================================
// TestCrossDBGetNoRows — verify querying a non-existent record returns sql.ErrNoRows
// ========================================================
func TestCrossDBGetNoRows(t *testing.T) {
	runWithSchema(crossSchema, t, func(db *sqlex.DB, t *testing.T, now string) {
		crossDBOnly(t)
		seedCrossData(db, t)

		var user CrossUser
		err := db.Get(&user, "SELECT * FROM cross_users WHERE name = ?", "NonExistent")
		if !errors.Is(err, sql.ErrNoRows) {
			t.Errorf("[%s] expected sql.ErrNoRows, got %v", dbLabel(db), err)
		}
	})
}

// ========================================================
// TestCrossDBAutoIN — verify Select/Get auto-expands IN clause when slice parameter is passed
// ========================================================
func TestCrossDBAutoIN(t *testing.T) {
	runWithSchema(crossSchema, t, func(db *sqlex.DB, t *testing.T, now string) {
		crossDBOnly(t)
		seedCrossData(db, t)

		// Select auto IN expansion
		var users []CrossUser
		err := db.Select(&users, "SELECT * FROM cross_users WHERE name IN (?) ORDER BY name",
			[]string{"Alice", "Charlie"})
		if err != nil {
			t.Fatalf("[%s] Select auto-IN failed: %v", dbLabel(db), err)
		}
		if len(users) != 2 {
			t.Fatalf("[%s] expected 2 users, got %d", dbLabel(db), len(users))
		}
		if users[0].Name != "Alice" || users[1].Name != "Charlie" {
			t.Errorf("[%s] unexpected: %s, %s", dbLabel(db), users[0].Name, users[1].Name)
		}

		// Select auto IN expansion (int slice)
		var orders []CrossOrder
		err = db.Select(&orders, "SELECT * FROM cross_orders WHERE user_id IN (?) ORDER BY id",
			[]int{1, 2})
		if err != nil {
			t.Fatalf("[%s] Select auto-IN int slice failed: %v", dbLabel(db), err)
		}
		if len(orders) != 3 {
			t.Errorf("[%s] expected 3 orders, got %d", dbLabel(db), len(orders))
		}
	})
}

// ========================================================
// TestCrossDBLastInsertId — verify LastInsertId differences
// ========================================================
func TestCrossDBLastInsertId(t *testing.T) {
	runWithSchema(crossSchema, t, func(db *sqlex.DB, t *testing.T, now string) {
		crossDBOnly(t)

		result, err := db.Exec(
			"INSERT INTO cross_users (name, email, age) VALUES ('LastIDTest', 'lid@test.com', 20)")
		if err != nil {
			t.Fatalf("[%s] Insert failed: %v", dbLabel(db), err)
		}

		if isMySQL(db) {
			id, err := result.LastInsertId()
			if err != nil {
				t.Fatalf("[MYSQL] LastInsertId failed: %v", err)
			}
			if id <= 0 {
				t.Errorf("[MYSQL] expected positive LastInsertId, got %d", id)
			}
		}

		if isPostgres(db) {
			_, err := result.LastInsertId()
			if err == nil {
				t.Logf("[POSTGRES] LastInsertId unexpectedly succeeded (driver-dependent)")
			} else {
				t.Logf("[POSTGRES] LastInsertId correctly returned error: %v", err)
			}
		}
	})
}

// ========================================================
// TestCrossDBQueryx — verify Queryx result set iteration
// ========================================================
func TestCrossDBQueryx(t *testing.T) {
	runWithSchema(crossSchema, t, func(db *sqlex.DB, t *testing.T, now string) {
		crossDBOnly(t)
		seedCrossData(db, t)

		// StructScan
		t.Run("StructScan", func(t *testing.T) {
			rows, err := db.Queryx("SELECT * FROM cross_users ORDER BY id")
			if err != nil {
				t.Fatalf("[%s] Queryx failed: %v", dbLabel(db), err)
			}
			defer rows.Close()

			var users []CrossUser
			for rows.Next() {
				var u CrossUser
				err := rows.StructScan(&u)
				if err != nil {
					t.Fatalf("[%s] StructScan failed: %v", dbLabel(db), err)
				}
				users = append(users, u)
			}
			if len(users) != 3 {
				t.Errorf("[%s] expected 3 users, got %d", dbLabel(db), len(users))
			}
		})

		// MapScan
		t.Run("MapScan", func(t *testing.T) {
			rows, err := db.Queryx(selectTop1(db, "SELECT name, email FROM cross_users ORDER BY id LIMIT 1"))
			if err != nil {
				t.Fatalf("[%s] Queryx failed: %v", dbLabel(db), err)
			}
			defer rows.Close()

			if rows.Next() {
				m := map[string]any{}
				err := rows.MapScan(m)
				if err != nil {
					t.Fatalf("[%s] MapScan failed: %v", dbLabel(db), err)
				}
				// MapScan return value types depend on the driver
				t.Logf("[%s] MapScan result: %v", dbLabel(db), m)
			}
		})

		// SliceScan
		t.Run("SliceScan", func(t *testing.T) {
			rows, err := db.Queryx(selectTop1(db, "SELECT name, email FROM cross_users ORDER BY id LIMIT 1"))
			if err != nil {
				t.Fatalf("[%s] Queryx failed: %v", dbLabel(db), err)
			}
			defer rows.Close()

			if rows.Next() {
				cols, err := rows.SliceScan()
				if err != nil {
					t.Fatalf("[%s] SliceScan failed: %v", dbLabel(db), err)
				}
				if len(cols) != 2 {
					t.Errorf("[%s] expected 2 columns, got %d", dbLabel(db), len(cols))
				}
			}
		})

		// QueryRowx + StructScan
		t.Run("QueryRowx", func(t *testing.T) {
			var user CrossUser
			err := db.QueryRowx("SELECT * FROM cross_users WHERE name = ?", "Alice").StructScan(&user)
			if err != nil {
				t.Fatalf("[%s] QueryRowx.StructScan failed: %v", dbLabel(db), err)
			}
			if user.Name != "Alice" {
				t.Errorf("[%s] expected Alice, got %s", dbLabel(db), user.Name)
			}
		})
	})
}

// ========================================================
// TestCrossDBContextAPIs — verify Context version APIs
// ========================================================
func TestCrossDBContextAPIs(t *testing.T) {
	runWithSchema(crossSchema, t, func(db *sqlex.DB, t *testing.T, now string) {
		crossDBOnly(t)
		seedCrossData(db, t)
		ctx := context.Background()

		// SelectContext
		var users []CrossUser
		err := db.SelectContext(ctx, &users, "SELECT * FROM cross_users ORDER BY id")
		if err != nil {
			t.Fatalf("[%s] SelectContext failed: %v", dbLabel(db), err)
		}
		if len(users) != 3 {
			t.Errorf("[%s] expected 3 users, got %d", dbLabel(db), len(users))
		}

		// GetContext
		var user CrossUser
		err = db.GetContext(ctx, &user, "SELECT * FROM cross_users WHERE name = ?", "Bob")
		if err != nil {
			t.Fatalf("[%s] GetContext failed: %v", dbLabel(db), err)
		}
		if user.Age != 25 {
			t.Errorf("[%s] expected age 25, got %d", dbLabel(db), user.Age)
		}

		// ExecContext
		result, err := db.ExecContext(ctx, "UPDATE cross_users SET age = ? WHERE name = ?", 26, "Bob")
		if err != nil {
			t.Fatalf("[%s] ExecContext failed: %v", dbLabel(db), err)
		}
		rowsAff, _ := result.RowsAffected()
		if rowsAff != 1 {
			t.Errorf("[%s] expected 1 row affected, got %d", dbLabel(db), rowsAff)
		}
	})
}

// ========================================================
// TestCrossDBContextTimeout — verify context timeout returns error
// ========================================================
func TestCrossDBContextTimeout(t *testing.T) {
	runWithSchema(crossSchema, t, func(db *sqlex.DB, t *testing.T, now string) {
		crossDBOnly(t)
		seedCrossData(db, t)

		// Use an already-expired context
		ctx, cancel := context.WithTimeout(context.Background(), 1*time.Nanosecond)
		defer cancel()

		// Wait briefly to ensure context has expired
		time.Sleep(1 * time.Millisecond)

		var users []CrossUser
		err := db.SelectContext(ctx, &users, "SELECT * FROM cross_users")
		if err == nil {
			t.Errorf("[%s] expected timeout error, got nil", dbLabel(db))
		} else {
			t.Logf("[%s] timeout error: %v", dbLabel(db), err)
		}
	})
}
