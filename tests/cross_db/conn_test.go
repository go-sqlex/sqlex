// conn_test.go — cross-database integration tests for Conn type
package cross_db_test

import (
	"context"
	"testing"

	sqlex "github.com/go-sqlex/sqlex"
)

// ========================================================
// TestCrossDBConn — 验证 Conn 的基础 Context API
// ========================================================
func TestCrossDBConn(t *testing.T) {
	runWithSchema(crossSchema, t, func(db *sqlex.DB, t *testing.T, now string) {
		crossDBOnly(t)
		seedCrossData(db, t)

		ctx := context.Background()
		conn, err := db.Connx(ctx)
		if err != nil {
			t.Fatalf("[%s] Connx failed: %v", dbLabel(db), err)
		}
		defer conn.Close()

		// SelectContext
		t.Run("SelectContext", func(t *testing.T) {
			var users []CrossUser
			err := conn.SelectContext(ctx, &users, "SELECT * FROM cross_users ORDER BY id")
			if err != nil {
				t.Fatalf("[%s] Conn.SelectContext failed: %v", dbLabel(db), err)
			}
			if len(users) != 3 {
				t.Errorf("[%s] expected 3 users, got %d", dbLabel(db), len(users))
			}
		})

		// GetContext
		t.Run("GetContext", func(t *testing.T) {
			var user CrossUser
			err := conn.GetContext(ctx, &user, "SELECT * FROM cross_users WHERE name = ?", "Alice")
			if err != nil {
				t.Fatalf("[%s] Conn.GetContext failed: %v", dbLabel(db), err)
			}
			if user.Email != "alice@example.com" {
				t.Errorf("[%s] expected alice@example.com, got %s", dbLabel(db), user.Email)
			}
		})

		// ExecContext
		t.Run("ExecContext", func(t *testing.T) {
			result, err := conn.ExecContext(ctx, "UPDATE cross_users SET age = ? WHERE name = ?", 31, "Alice")
			if err != nil {
				t.Fatalf("[%s] Conn.ExecContext failed: %v", dbLabel(db), err)
			}
			rows, _ := result.RowsAffected()
			if rows != 1 {
				t.Errorf("[%s] expected 1 row affected, got %d", dbLabel(db), rows)
			}
		})

		// QueryxContext
		t.Run("QueryxContext", func(t *testing.T) {
			rows, err := conn.QueryxContext(ctx, "SELECT * FROM cross_users ORDER BY id")
			if err != nil {
				t.Fatalf("[%s] Conn.QueryxContext failed: %v", dbLabel(db), err)
			}
			defer rows.Close()

			count := 0
			for rows.Next() {
				var u CrossUser
				err := rows.StructScan(&u)
				if err != nil {
					t.Fatalf("[%s] StructScan failed: %v", dbLabel(db), err)
				}
				count++
			}
			if count != 3 {
				t.Errorf("[%s] expected 3 users, got %d", dbLabel(db), count)
			}
		})

		// QueryRowxContext
		t.Run("QueryRowxContext", func(t *testing.T) {
			var user CrossUser
			err := conn.QueryRowxContext(ctx, "SELECT * FROM cross_users WHERE name = ?", "Bob").StructScan(&user)
			if err != nil {
				t.Fatalf("[%s] Conn.QueryRowxContext failed: %v", dbLabel(db), err)
			}
			if user.Age != 25 {
				t.Errorf("[%s] expected age 25, got %d", dbLabel(db), user.Age)
			}
		})
	})
}

// ========================================================
// TestCrossDBConnNamed — 验证 Conn 的命名参数 Context API
// ========================================================
func TestCrossDBConnNamed(t *testing.T) {
	runWithSchema(crossSchema, t, func(db *sqlex.DB, t *testing.T, now string) {
		crossDBOnly(t)
		seedCrossData(db, t)

		ctx := context.Background()
		conn, err := db.Connx(ctx)
		if err != nil {
			t.Fatalf("[%s] Connx failed: %v", dbLabel(db), err)
		}
		defer conn.Close()

		// NamedGetContext
		t.Run("NamedGetContext", func(t *testing.T) {
			var user CrossUser
			err := conn.NamedGetContext(ctx, &user,
				`SELECT * FROM cross_users WHERE name = :name`,
				map[string]any{"name": "Alice"})
			if err != nil {
				t.Fatalf("[%s] Conn.NamedGetContext failed: %v", dbLabel(db), err)
			}
			if user.Email != "alice@example.com" {
				t.Errorf("[%s] expected alice@example.com, got %s", dbLabel(db), user.Email)
			}
		})

		// NamedSelectContext
		t.Run("NamedSelectContext", func(t *testing.T) {
			var users []CrossUser
			err := conn.NamedSelectContext(ctx, &users,
				`SELECT * FROM cross_users WHERE age > :min_age ORDER BY age`,
				map[string]any{"min_age": 26})
			if err != nil {
				t.Fatalf("[%s] Conn.NamedSelectContext failed: %v", dbLabel(db), err)
			}
			if len(users) != 2 {
				t.Errorf("[%s] expected 2 users, got %d", dbLabel(db), len(users))
			}
		})

		// NamedExecContext
		t.Run("NamedExecContext", func(t *testing.T) {
			result, err := conn.NamedExecContext(ctx,
				`INSERT INTO cross_users (name, email, age) VALUES (:name, :email, :age)`,
				map[string]any{"name": "ConnUser", "email": "conn@example.com", "age": 28})
			if err != nil {
				t.Fatalf("[%s] Conn.NamedExecContext failed: %v", dbLabel(db), err)
			}
			rows, _ := result.RowsAffected()
			if rows != 1 {
				t.Errorf("[%s] expected 1 row affected, got %d", dbLabel(db), rows)
			}
		})
	})
}

// ========================================================
// TestCrossDBConnNamedIN — 验证 Conn + NamedSelect + IN 展开
// ========================================================
func TestCrossDBConnNamedIN(t *testing.T) {
	runWithSchema(crossSchema, t, func(db *sqlex.DB, t *testing.T, now string) {
		crossDBOnly(t)
		seedCrossData(db, t)

		ctx := context.Background()
		conn, err := db.Connx(ctx)
		if err != nil {
			t.Fatalf("[%s] Connx failed: %v", dbLabel(db), err)
		}
		defer conn.Close()

		var users []CrossUser
		err = conn.NamedSelectContext(ctx, &users,
			`SELECT * FROM cross_users WHERE name IN (:names) ORDER BY name`,
			map[string]any{"names": []string{"Alice", "Charlie"}})
		if err != nil {
			t.Fatalf("[%s] Conn.NamedSelectContext IN failed: %v", dbLabel(db), err)
		}
		if len(users) != 2 {
			t.Fatalf("[%s] expected 2 users, got %d", dbLabel(db), len(users))
		}
		if users[0].Name != "Alice" || users[1].Name != "Charlie" {
			t.Errorf("[%s] unexpected users: %s, %s", dbLabel(db), users[0].Name, users[1].Name)
		}
	})
}
