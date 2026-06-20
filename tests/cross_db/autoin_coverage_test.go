// autoin_coverage_test.go — Verify autoIn is fully covered on Exec/Queryx/QueryRowx/MustExec/NamedQuery
// paths (including DB/Tx/Conn hosts).
//
// Background: Early implementations only connected autoIn on Select/Get/NamedExec/NamedGet/NamedSelect paths,
// missing Exec/Queryx/QueryRowx/MustExec/NamedQuery paths. This test suite serves as a regression-prevention contract.
package cross_db_test

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	sqlex "github.com/go-sqlex/sqlex"
)

// ========================================================
// DB paths
// ========================================================

// TestCrossDB_DB_Exec_AutoIN — verify db.Exec("... IN (?)", []int) auto-expansion
func TestCrossDB_DB_Exec_AutoIN(t *testing.T) {
	runWithSchema(crossSchema, t, func(db *sqlex.DB, t *testing.T, now string) {
		crossDBOnly(t)
		seedCrossData(db, t)

		result, err := db.Exec(
			"DELETE FROM cross_orders WHERE user_id IN (?)", []int{1, 2})
		if err != nil {
			t.Fatalf("[%s] DB.Exec autoIN failed: %v", dbLabel(db), err)
		}
		affected, err := result.RowsAffected()
		if err != nil {
			t.Fatalf("[%s] RowsAffected failed: %v", dbLabel(db), err)
		}
		if affected != 3 {
			t.Errorf("[%s] expected 3 rows affected, got %d", dbLabel(db), affected)
		}
	})
}

// TestCrossDB_DB_Exec_NoSlice_FastPath — verify non-slice arguments don't break normal Exec
func TestCrossDB_DB_Exec_NoSlice_FastPath(t *testing.T) {
	runWithSchema(crossSchema, t, func(db *sqlex.DB, t *testing.T, now string) {
		crossDBOnly(t)
		seedCrossData(db, t)

		result, err := db.Exec(
			"UPDATE cross_users SET age = ? WHERE name = ?", 99, "Alice")
		if err != nil {
			t.Fatalf("[%s] DB.Exec no-slice fast path failed: %v", dbLabel(db), err)
		}
		affected, _ := result.RowsAffected()
		if affected != 1 {
			t.Errorf("[%s] expected 1 row affected, got %d", dbLabel(db), affected)
		}
	})
}

// TestCrossDB_DB_ExecContext_AutoIN — Context version positional parameter IN expansion
func TestCrossDB_DB_ExecContext_AutoIN(t *testing.T) {
	runWithSchema(crossSchema, t, func(db *sqlex.DB, t *testing.T, now string) {
		crossDBOnly(t)
		seedCrossData(db, t)

		ctx := context.Background()
		_, err := db.ExecContext(ctx,
			"DELETE FROM cross_orders WHERE status IN (?)", []string{"paid", "pending"})
		if err != nil {
			t.Fatalf("[%s] DB.ExecContext autoIN failed: %v", dbLabel(db), err)
		}
	})
}

// TestCrossDB_DB_Queryx_AutoIN — Queryx + IN expansion
func TestCrossDB_DB_Queryx_AutoIN(t *testing.T) {
	runWithSchema(crossSchema, t, func(db *sqlex.DB, t *testing.T, now string) {
		crossDBOnly(t)
		seedCrossData(db, t)

		rows, err := db.Queryx(
			"SELECT * FROM cross_users WHERE name IN (?) ORDER BY name",
			[]string{"Alice", "Bob"})
		if err != nil {
			t.Fatalf("[%s] DB.Queryx autoIN failed: %v", dbLabel(db), err)
		}
		defer rows.Close()

		var users []CrossUser
		for rows.Next() {
			var u CrossUser
			if err := rows.StructScan(&u); err != nil {
				t.Fatalf("[%s] StructScan failed: %v", dbLabel(db), err)
			}
			users = append(users, u)
		}
		if len(users) != 2 {
			t.Errorf("[%s] expected 2 users, got %d", dbLabel(db), len(users))
		}
	})
}

// TestCrossDB_DB_QueryRowx_AutoIN — QueryRowx single row + IN
func TestCrossDB_DB_QueryRowx_AutoIN(t *testing.T) {
	runWithSchema(crossSchema, t, func(db *sqlex.DB, t *testing.T, now string) {
		crossDBOnly(t)
		seedCrossData(db, t)

		var u CrossUser
		q := selectTop1(db, "SELECT * FROM cross_users WHERE name IN (?) AND age > ? ORDER BY age DESC LIMIT 1")
		err := db.QueryRowx(q,
			[]string{"Alice", "Charlie"}, 0).StructScan(&u)
		if err != nil {
			t.Fatalf("[%s] DB.QueryRowx autoIN failed: %v", dbLabel(db), err)
		}
		if u.Name != "Charlie" {
			t.Errorf("[%s] expected Charlie, got %s", dbLabel(db), u.Name)
		}
	})
}

// TestCrossDB_DB_QueryRowx_AutoIN_EmptySliceErr — empty slice should propagate autoIn error to Row.err
func TestCrossDB_DB_QueryRowx_AutoIN_EmptySliceErr(t *testing.T) {
	runWithSchema(crossSchema, t, func(db *sqlex.DB, t *testing.T, now string) {
		crossDBOnly(t)
		seedCrossData(db, t)

		// Pass empty slice, autoIn (internal In) will report "empty slice passed to 'in' query"
		var u CrossUser
		err := db.QueryRowx(
			"SELECT * FROM cross_users WHERE name IN (?)", []string{}).StructScan(&u)
		if err == nil {
			t.Fatalf("[%s] expected error for empty slice, got nil", dbLabel(db))
		}
		// Verify it's not a downstream error like sql.ErrNoRows, but the expected autoIn error
		if errors.Is(err, sql.ErrNoRows) {
			t.Errorf("[%s] expected autoIn error, got sql.ErrNoRows: %v", dbLabel(db), err)
		}
	})
}

// TestCrossDB_DB_MustExec_AutoIN — MustExec delegates to Exec, should auto-follow
func TestCrossDB_DB_MustExec_AutoIN(t *testing.T) {
	runWithSchema(crossSchema, t, func(db *sqlex.DB, t *testing.T, now string) {
		crossDBOnly(t)
		seedCrossData(db, t)

		defer func() {
			if r := recover(); r != nil {
				t.Fatalf("[%s] DB.MustExec autoIN unexpectedly panicked: %v", dbLabel(db), r)
			}
		}()

		result := db.MustExec(
			"DELETE FROM cross_orders WHERE user_id IN (?)", []int{1, 2})
		affected, _ := result.RowsAffected()
		if affected != 3 {
			t.Errorf("[%s] expected 3 rows affected, got %d", dbLabel(db), affected)
		}
	})
}

// TestCrossDB_DB_NamedQuery_AutoIN — db.NamedQuery + IN (:ids) slice expansion
func TestCrossDB_DB_NamedQuery_AutoIN(t *testing.T) {
	runWithSchema(crossSchema, t, func(db *sqlex.DB, t *testing.T, now string) {
		crossDBOnly(t)
		seedCrossData(db, t)

		rows, err := db.NamedQuery(
			"SELECT * FROM cross_users WHERE name IN (:names) ORDER BY name",
			map[string]any{"names": []string{"Alice", "Bob"}})
		if err != nil {
			t.Fatalf("[%s] DB.NamedQuery autoIN failed: %v", dbLabel(db), err)
		}
		defer rows.Close()

		var users []CrossUser
		for rows.Next() {
			var u CrossUser
			if err := rows.StructScan(&u); err != nil {
				t.Fatalf("[%s] StructScan failed: %v", dbLabel(db), err)
			}
			users = append(users, u)
		}
		if len(users) != 2 {
			t.Errorf("[%s] expected 2 users, got %d", dbLabel(db), len(users))
		}
	})
}

// TestCrossDB_DB_NamedQueryContext_AutoIN — Context version
func TestCrossDB_DB_NamedQueryContext_AutoIN(t *testing.T) {
	runWithSchema(crossSchema, t, func(db *sqlex.DB, t *testing.T, now string) {
		crossDBOnly(t)
		seedCrossData(db, t)

		ctx := context.Background()
		rows, err := db.NamedQueryContext(ctx,
			"SELECT * FROM cross_users WHERE age IN (:ages) ORDER BY age",
			map[string]any{"ages": []int{25, 30}})
		if err != nil {
			t.Fatalf("[%s] DB.NamedQueryContext autoIN failed: %v", dbLabel(db), err)
		}
		defer rows.Close()

		count := 0
		for rows.Next() {
			count++
		}
		if count != 2 {
			t.Errorf("[%s] expected 2 rows, got %d", dbLabel(db), count)
		}
	})
}

// ========================================================
// Tx paths
// ========================================================

// TestCrossDB_Tx_Exec_AutoIN — Tx.Exec + IN
func TestCrossDB_Tx_Exec_AutoIN(t *testing.T) {
	runWithSchema(crossSchema, t, func(db *sqlex.DB, t *testing.T, now string) {
		crossDBOnly(t)
		seedCrossData(db, t)

		tx, err := db.Beginx()
		if err != nil {
			t.Fatalf("[%s] Beginx failed: %v", dbLabel(db), err)
		}
		defer tx.Rollback()

		_, err = tx.Exec(
			"DELETE FROM cross_orders WHERE user_id IN (?)", []int{1, 2})
		if err != nil {
			t.Fatalf("[%s] Tx.Exec autoIN failed: %v", dbLabel(db), err)
		}
	})
}

// TestCrossDB_Tx_Queryx_AutoIN
func TestCrossDB_Tx_Queryx_AutoIN(t *testing.T) {
	runWithSchema(crossSchema, t, func(db *sqlex.DB, t *testing.T, now string) {
		crossDBOnly(t)
		seedCrossData(db, t)

		tx, err := db.Beginx()
		if err != nil {
			t.Fatalf("[%s] Beginx failed: %v", dbLabel(db), err)
		}
		defer tx.Rollback()

		rows, err := tx.Queryx(
			"SELECT * FROM cross_users WHERE name IN (?)",
			[]string{"Alice", "Bob"})
		if err != nil {
			t.Fatalf("[%s] Tx.Queryx autoIN failed: %v", dbLabel(db), err)
		}
		defer rows.Close()

		count := 0
		for rows.Next() {
			count++
		}
		if count != 2 {
			t.Errorf("[%s] expected 2 rows, got %d", dbLabel(db), count)
		}
	})
}

// TestCrossDB_Tx_QueryRowx_AutoIN
func TestCrossDB_Tx_QueryRowx_AutoIN(t *testing.T) {
	runWithSchema(crossSchema, t, func(db *sqlex.DB, t *testing.T, now string) {
		crossDBOnly(t)
		seedCrossData(db, t)

		tx, err := db.Beginx()
		if err != nil {
			t.Fatalf("[%s] Beginx failed: %v", dbLabel(db), err)
		}
		defer tx.Rollback()

		var u CrossUser
		q := selectTop1(db, "SELECT * FROM cross_users WHERE name IN (?) ORDER BY name LIMIT 1")
		err = tx.QueryRowx(q,
			[]string{"Bob", "Charlie"}).StructScan(&u)
		if err != nil {
			t.Fatalf("[%s] Tx.QueryRowx autoIN failed: %v", dbLabel(db), err)
		}
		if u.Name != "Bob" {
			t.Errorf("[%s] expected Bob, got %s", dbLabel(db), u.Name)
		}
	})
}

// TestCrossDB_Tx_NamedQuery_AutoIN
func TestCrossDB_Tx_NamedQuery_AutoIN(t *testing.T) {
	runWithSchema(crossSchema, t, func(db *sqlex.DB, t *testing.T, now string) {
		crossDBOnly(t)
		seedCrossData(db, t)

		tx, err := db.Beginx()
		if err != nil {
			t.Fatalf("[%s] Beginx failed: %v", dbLabel(db), err)
		}
		defer tx.Rollback()

		rows, err := tx.NamedQuery(
			"SELECT * FROM cross_users WHERE name IN (:names) ORDER BY name",
			map[string]any{"names": []string{"Alice", "Charlie"}})
		if err != nil {
			t.Fatalf("[%s] Tx.NamedQuery autoIN failed: %v", dbLabel(db), err)
		}
		defer rows.Close()

		count := 0
		for rows.Next() {
			count++
		}
		if count != 2 {
			t.Errorf("[%s] expected 2 rows, got %d", dbLabel(db), count)
		}
	})
}

// ========================================================
// Conn paths
// ========================================================

// TestCrossDB_Conn_Exec_AutoIN
func TestCrossDB_Conn_Exec_AutoIN(t *testing.T) {
	runWithSchema(crossSchema, t, func(db *sqlex.DB, t *testing.T, now string) {
		crossDBOnly(t)
		seedCrossData(db, t)

		ctx := context.Background()
		conn, err := db.Connx(ctx)
		if err != nil {
			t.Fatalf("[%s] Connx failed: %v", dbLabel(db), err)
		}
		defer conn.Close()

		_, err = conn.ExecContext(ctx,
			"DELETE FROM cross_orders WHERE user_id IN (?)", []int{1, 2})
		if err != nil {
			t.Fatalf("[%s] Conn.ExecContext autoIN failed: %v", dbLabel(db), err)
		}
	})
}

// TestCrossDB_Conn_Queryx_AutoIN
func TestCrossDB_Conn_Queryx_AutoIN(t *testing.T) {
	runWithSchema(crossSchema, t, func(db *sqlex.DB, t *testing.T, now string) {
		crossDBOnly(t)
		seedCrossData(db, t)

		ctx := context.Background()
		conn, err := db.Connx(ctx)
		if err != nil {
			t.Fatalf("[%s] Connx failed: %v", dbLabel(db), err)
		}
		defer conn.Close()

		rows, err := conn.QueryxContext(ctx,
			"SELECT * FROM cross_users WHERE name IN (?)",
			[]string{"Alice", "Bob"})
		if err != nil {
			t.Fatalf("[%s] Conn.QueryxContext autoIN failed: %v", dbLabel(db), err)
		}
		defer rows.Close()

		count := 0
		for rows.Next() {
			count++
		}
		if count != 2 {
			t.Errorf("[%s] expected 2 rows, got %d", dbLabel(db), count)
		}
	})
}

// TestCrossDB_Conn_QueryRowx_AutoIN
func TestCrossDB_Conn_QueryRowx_AutoIN(t *testing.T) {
	runWithSchema(crossSchema, t, func(db *sqlex.DB, t *testing.T, now string) {
		crossDBOnly(t)
		seedCrossData(db, t)

		ctx := context.Background()
		conn, err := db.Connx(ctx)
		if err != nil {
			t.Fatalf("[%s] Connx failed: %v", dbLabel(db), err)
		}
		defer conn.Close()

		var u CrossUser
		q := selectTop1(db, "SELECT * FROM cross_users WHERE name IN (?) ORDER BY name LIMIT 1")
		err = conn.QueryRowxContext(ctx, q,
			[]string{"Alice", "Charlie"}).StructScan(&u)
		if err != nil {
			t.Fatalf("[%s] Conn.QueryRowxContext autoIN failed: %v", dbLabel(db), err)
		}
		if u.Name != "Alice" {
			t.Errorf("[%s] expected Alice, got %s", dbLabel(db), u.Name)
		}
	})
}

// TestCrossDB_Conn_NamedQuery_AutoIN
func TestCrossDB_Conn_NamedQuery_AutoIN(t *testing.T) {
	runWithSchema(crossSchema, t, func(db *sqlex.DB, t *testing.T, now string) {
		crossDBOnly(t)
		seedCrossData(db, t)

		ctx := context.Background()
		conn, err := db.Connx(ctx)
		if err != nil {
			t.Fatalf("[%s] Connx failed: %v", dbLabel(db), err)
		}
		defer conn.Close()

		rows, err := conn.NamedQueryContext(ctx,
			"SELECT * FROM cross_users WHERE name IN (:names) ORDER BY name",
			map[string]any{"names": []string{"Bob", "Charlie"}})
		if err != nil {
			t.Fatalf("[%s] Conn.NamedQueryContext autoIN failed: %v", dbLabel(db), err)
		}
		defer rows.Close()

		count := 0
		for rows.Next() {
			count++
		}
		if count != 2 {
			t.Errorf("[%s] expected 2 rows, got %d", dbLabel(db), count)
		}
	})
}

// ========================================================
// Top-level function paths (bypass DB/Tx/Conn, directly use sqlex.NamedQueryContext etc.)
// ========================================================

// TestCrossDB_TopLevel_NamedQueryContext_AutoIN — verify top-level sqlex.NamedQueryContext also injects autoIn
func TestCrossDB_TopLevel_NamedQueryContext_AutoIN(t *testing.T) {
	runWithSchema(crossSchema, t, func(db *sqlex.DB, t *testing.T, now string) {
		crossDBOnly(t)
		seedCrossData(db, t)

		ctx := context.Background()
		rows, err := sqlex.NamedQueryContext(ctx, db,
			"SELECT * FROM cross_users WHERE name IN (:names)",
			map[string]any{"names": []string{"Alice", "Bob"}})
		if err != nil {
			t.Fatalf("[%s] sqlex.NamedQueryContext autoIN failed: %v", dbLabel(db), err)
		}
		defer rows.Close()

		count := 0
		for rows.Next() {
			count++
		}
		if count != 2 {
			t.Errorf("[%s] expected 2 rows, got %d", dbLabel(db), count)
		}
	})
}

// TestCrossDB_TopLevel_NamedExecContext_AutoIN
func TestCrossDB_TopLevel_NamedExecContext_AutoIN(t *testing.T) {
	runWithSchema(crossSchema, t, func(db *sqlex.DB, t *testing.T, now string) {
		crossDBOnly(t)
		seedCrossData(db, t)

		ctx := context.Background()
		_, err := sqlex.NamedExecContext(ctx, db,
			"DELETE FROM cross_orders WHERE user_id IN (:ids)",
			map[string]any{"ids": []int{1, 2}})
		if err != nil {
			t.Fatalf("[%s] sqlex.NamedExecContext autoIN failed: %v", dbLabel(db), err)
		}
	})
}

// ========================================================
// Contract-level tests: Assert DB/Tx/Conn all fulfill the autoIn contract via BindExt / NamedExt interfaces
//
// These tests are more regression-resistant than method-level tests — if a new host is added in the future,
// as long as it declares
//
//	_ BindExt = (*New)(nil)
//
// and appears in the contractCases list, it will be automatically covered.
// ========================================================

// TestCrossDB_BindExt_Contract_ExecAutoIN — all BindExt implementations' Exec must support IN slice expansion
func TestCrossDB_BindExt_Contract_ExecAutoIN(t *testing.T) {
	runWithSchema(crossSchema, t, func(db *sqlex.DB, t *testing.T, now string) {
		crossDBOnly(t)

		ctx := context.Background()
		// contractCases lists factories for all current BindExt implementations
		contractCases := []struct {
			name    string
			factory func(t *testing.T) (sqlex.BindExt, func())
		}{
			{
				name: "DB",
				factory: func(t *testing.T) (sqlex.BindExt, func()) {
					return db, func() {}
				},
			},
			{
				name: "Tx",
				factory: func(t *testing.T) (sqlex.BindExt, func()) {
					tx, err := db.Beginx()
					if err != nil {
						t.Fatalf("Beginx failed: %v", err)
					}
					return tx, func() { _ = tx.Rollback() }
				},
			},
			{
				name: "Conn",
				factory: func(t *testing.T) (sqlex.BindExt, func()) {
					conn, err := db.Connx(ctx)
					if err != nil {
						t.Fatalf("Connx failed: %v", err)
					}
					return conn, func() { _ = conn.Close() }
				},
			},
		}

		for _, c := range contractCases {
			t.Run(c.name, func(t *testing.T) {
				// Each case independently prepares data to avoid mutual interference
				multiExec(db, crossSchema.Drop)
				create, _, _ := schemaForDB(db, crossSchema)
				multiExec(db, create)
				seedCrossData(db, t)

				ext, cleanup := c.factory(t)
				defer cleanup()

				// Contract 1: Exec supports IN slice expansion
				_, err := ext.Exec(
					"UPDATE cross_users SET age = ? WHERE name IN (?)",
					100, []string{"Alice", "Bob"})
				if err != nil {
					t.Fatalf("[%s/%s] Exec autoIN contract failed: %v",
						dbLabel(db), c.name, err)
				}

				// Contract 2: Queryx supports IN slice expansion
				rows, err := ext.Queryx(
					"SELECT * FROM cross_users WHERE name IN (?)",
					[]string{"Alice", "Bob"})
				if err != nil {
					t.Fatalf("[%s/%s] Queryx autoIN contract failed: %v",
						dbLabel(db), c.name, err)
				}
				rows.Close()

				// Contract 3: QueryRowx supports IN slice expansion (error stuffed into Row.err)
				q := selectTop1(db, "SELECT name FROM cross_users WHERE name IN (?) LIMIT 1")
				row := ext.QueryRowx(q,
					[]string{"Alice"})
				var name string
				if err := row.Scan(&name); err != nil {
					t.Fatalf("[%s/%s] QueryRowx autoIN contract failed: %v",
						dbLabel(db), c.name, err)
				}
				if name != "Alice" {
					t.Errorf("[%s/%s] expected Alice, got %s",
						dbLabel(db), c.name, name)
				}
			})
		}
	})
}

// TestCrossDB_NamedExt_Contract_AutoIN — all NamedExt implementations' Named* methods must support IN slice expansion
func TestCrossDB_NamedExt_Contract_AutoIN(t *testing.T) {
	runWithSchema(crossSchema, t, func(db *sqlex.DB, t *testing.T, now string) {
		crossDBOnly(t)

		ctx := context.Background()
		contractCases := []struct {
			name    string
			factory func(t *testing.T) (sqlex.NamedExt, func())
		}{
			{
				name: "DB",
				factory: func(t *testing.T) (sqlex.NamedExt, func()) {
					return db, func() {}
				},
			},
			{
				name: "Tx",
				factory: func(t *testing.T) (sqlex.NamedExt, func()) {
					tx, err := db.Beginx()
					if err != nil {
						t.Fatalf("Beginx failed: %v", err)
					}
					return tx, func() { _ = tx.Rollback() }
				},
			},
			{
				name: "Conn",
				factory: func(t *testing.T) (sqlex.NamedExt, func()) {
					conn, err := db.Connx(ctx)
					if err != nil {
						t.Fatalf("Connx failed: %v", err)
					}
					return conn, func() { _ = conn.Close() }
				},
			},
		}

		for _, c := range contractCases {
			t.Run(c.name, func(t *testing.T) {
				// Independent data preparation
				multiExec(db, crossSchema.Drop)
				create, _, _ := schemaForDB(db, crossSchema)
				multiExec(db, create)
				seedCrossData(db, t)

				ext, cleanup := c.factory(t)
				defer cleanup()

				// Contract 1: NamedExec supports IN slice expansion
				_, err := ext.NamedExec(
					"UPDATE cross_users SET age = :age WHERE name IN (:names)",
					map[string]any{"age": 200, "names": []string{"Alice", "Bob"}})
				if err != nil {
					t.Fatalf("[%s/%s] NamedExec autoIN contract failed: %v",
						dbLabel(db), c.name, err)
				}

				// Contract 2: NamedQuery supports IN slice expansion
				rows, err := ext.NamedQuery(
					"SELECT * FROM cross_users WHERE name IN (:names)",
					map[string]any{"names": []string{"Alice", "Bob"}})
				if err != nil {
					t.Fatalf("[%s/%s] NamedQuery autoIN contract failed: %v",
						dbLabel(db), c.name, err)
				}
				rows.Close()

				// Contract 3: NamedSelect supports IN slice expansion
				var users []CrossUser
				err = ext.NamedSelect(&users,
					"SELECT * FROM cross_users WHERE name IN (:names) ORDER BY name",
					map[string]any{"names": []string{"Alice", "Bob"}})
				if err != nil {
					t.Fatalf("[%s/%s] NamedSelect autoIN contract failed: %v",
						dbLabel(db), c.name, err)
				}
				if len(users) != 2 {
					t.Errorf("[%s/%s] expected 2 users, got %d",
						dbLabel(db), c.name, len(users))
				}
			})
		}
	})
}
