// prepared_test.go — cross-database integration tests for prepared statements and unified interfaces
package cross_db_test

import (
	"context"
	"testing"

	sqlex "github.com/go-sqlex/sqlex"
)

// ========================================================
// TestCrossDBPrepareNamed — 验证 PrepareNamed + NamedStmt 操作
// ========================================================
func TestCrossDBPrepareNamed(t *testing.T) {
	runWithSchema(crossSchema, t, func(db *sqlex.DB, t *testing.T, now string) {
		crossDBOnly(t)
		seedCrossData(db, t)

		// PrepareNamed
		nstmt, err := db.PrepareNamed(`SELECT * FROM cross_users WHERE name = :name`)
		if err != nil {
			t.Fatalf("[%s] PrepareNamed failed: %v", dbLabel(db), err)
		}
		defer nstmt.Close()

		// NamedStmt.Get
		t.Run("Get", func(t *testing.T) {
			var user CrossUser
			err := nstmt.Get(&user, map[string]any{"name": "Alice"})
			if err != nil {
				t.Fatalf("[%s] NamedStmt.Get failed: %v", dbLabel(db), err)
			}
			if user.Email != "alice@example.com" {
				t.Errorf("[%s] expected alice@example.com, got %s", dbLabel(db), user.Email)
			}
		})

		// NamedStmt.Select
		t.Run("Select", func(t *testing.T) {
			var users []CrossUser
			err := nstmt.Select(&users, map[string]any{"name": "Bob"})
			if err != nil {
				t.Fatalf("[%s] NamedStmt.Select failed: %v", dbLabel(db), err)
			}
			if len(users) != 1 {
				t.Errorf("[%s] expected 1 user, got %d", dbLabel(db), len(users))
			}
		})
	})
}

// ========================================================
// TestCrossDBNamedStmtExecResult — 验证 NamedStmt.Exec 返回非 nil Result
// ========================================================
func TestCrossDBNamedStmtExecResult(t *testing.T) {
	runWithSchema(crossSchema, t, func(db *sqlex.DB, t *testing.T, now string) {
		crossDBOnly(t)

		nstmt, err := db.PrepareNamed(
			`INSERT INTO cross_users (name, email, age) VALUES (:name, :email, :age)`)
		if err != nil {
			t.Fatalf("[%s] PrepareNamed failed: %v", dbLabel(db), err)
		}
		defer nstmt.Close()

		result, err := nstmt.Exec(map[string]any{
			"name": "PrepUser", "email": "prep@example.com", "age": 27})
		if err != nil {
			t.Fatalf("[%s] NamedStmt.Exec failed: %v", dbLabel(db), err)
		}
		if result == nil {
			t.Fatalf("[%s] NamedStmt.Exec returned nil Result (Bug regression)", dbLabel(db))
		}

		rows, err := result.RowsAffected()
		if err != nil {
			t.Fatalf("[%s] RowsAffected failed: %v", dbLabel(db), err)
		}
		if rows != 1 {
			t.Errorf("[%s] expected 1 row affected, got %d", dbLabel(db), rows)
		}
	})
}

// ========================================================
// TestCrossDBNamedStmtInTx — 验证 tx.NamedStmt 复用预编译语句
// ========================================================
func TestCrossDBNamedStmtInTx(t *testing.T) {
	runWithSchema(crossSchema, t, func(db *sqlex.DB, t *testing.T, now string) {
		crossDBOnly(t)
		seedCrossData(db, t)

		nstmt, err := db.PrepareNamed(`SELECT * FROM cross_users WHERE name = :name`)
		if err != nil {
			t.Fatalf("[%s] PrepareNamed failed: %v", dbLabel(db), err)
		}
		defer nstmt.Close()

		tx, err := db.Beginx()
		if err != nil {
			t.Fatalf("[%s] Beginx failed: %v", dbLabel(db), err)
		}
		defer tx.Rollback()

		// tx.NamedStmt 复用
		txStmt := tx.NamedStmt(nstmt)
		var user CrossUser
		err = txStmt.Get(&user, map[string]any{"name": "Alice"})
		if err != nil {
			t.Fatalf("[%s] tx.NamedStmt.Get failed: %v", dbLabel(db), err)
		}
		if user.Name != "Alice" {
			t.Errorf("[%s] expected Alice, got %s", dbLabel(db), user.Name)
		}
	})
}

// ========================================================
// TestCrossDBPreparex — 验证 Preparex 自动 Rebind，统一使用 ? 占位符
// ========================================================
func TestCrossDBPreparex(t *testing.T) {
	runWithSchema(crossSchema, t, func(db *sqlex.DB, t *testing.T, now string) {
		crossDBOnly(t)
		seedCrossData(db, t)

		// 统一使用 ? 占位符，Preparex 自动 Rebind 为目标数据库格式
		t.Run("Get", func(t *testing.T) {
			stmt, err := db.Preparex("SELECT * FROM cross_users WHERE name = ?")
			if err != nil {
				t.Fatalf("[%s] Preparex failed: %v", dbLabel(db), err)
			}
			defer stmt.Close()

			var user CrossUser
			err = stmt.Get(&user, "Alice")
			if err != nil {
				t.Fatalf("[%s] Preparex.Get failed: %v", dbLabel(db), err)
			}
			if user.Name != "Alice" {
				t.Errorf("[%s] expected Alice, got %s", dbLabel(db), user.Name)
			}
		})

		// 多参数预编译
		t.Run("Select_MultiParam", func(t *testing.T) {
			stmt, err := db.Preparex("SELECT * FROM cross_users WHERE age > ? ORDER BY age")
			if err != nil {
				t.Fatalf("[%s] Preparex multi-param failed: %v", dbLabel(db), err)
			}
			defer stmt.Close()

			var users []CrossUser
			err = stmt.Select(&users, 26)
			if err != nil {
				t.Fatalf("[%s] Preparex.Select failed: %v", dbLabel(db), err)
			}
			if len(users) != 2 {
				t.Errorf("[%s] expected 2 users with age > 26, got %d", dbLabel(db), len(users))
			}
		})

		// 事务中使用 Preparex
		t.Run("InTx", func(t *testing.T) {
			tx, err := db.Beginx()
			if err != nil {
				t.Fatalf("[%s] Beginx failed: %v", dbLabel(db), err)
			}
			defer tx.Rollback()

			stmt, err := tx.Preparex("SELECT * FROM cross_users WHERE name = ?")
			if err != nil {
				t.Fatalf("[%s] tx.Preparex failed: %v", dbLabel(db), err)
			}
			defer stmt.Close()

			var user CrossUser
			err = stmt.Get(&user, "Bob")
			if err != nil {
				t.Fatalf("[%s] tx.Preparex.Get failed: %v", dbLabel(db), err)
			}
			if user.Email != "bob@example.com" {
				t.Errorf("[%s] expected bob@example.com, got %s", dbLabel(db), user.Email)
			}
		})
	})
}

// ========================================================
// TestCrossDBPreparexContext — 验证 PreparexContext 自动 Rebind
// ========================================================
func TestCrossDBPreparexContext(t *testing.T) {
	runWithSchema(crossSchema, t, func(db *sqlex.DB, t *testing.T, now string) {
		crossDBOnly(t)
		seedCrossData(db, t)

		ctx := context.Background()

		// 统一使用 ? 占位符
		stmt, err := db.PreparexContext(ctx, "SELECT * FROM cross_users WHERE name = ?")
		if err != nil {
			t.Fatalf("[%s] PreparexContext failed: %v", dbLabel(db), err)
		}
		defer stmt.Close()

		var user CrossUser
		err = stmt.Get(&user, "Charlie")
		if err != nil {
			t.Fatalf("[%s] PreparexContext.Get failed: %v", dbLabel(db), err)
		}
		if user.Name != "Charlie" {
			t.Errorf("[%s] expected Charlie, got %s", dbLabel(db), user.Name)
		}
	})
}

// ========================================================
// TestCrossDBPreparerContextInterface — 验证 PreparerContext 接口
// 可以同时接受 DB、Tx、Conn，统一使用 ? 占位符
// ========================================================
func TestCrossDBPreparerContextInterface(t *testing.T) {
	runWithSchema(crossSchema, t, func(db *sqlex.DB, t *testing.T, now string) {
		crossDBOnly(t)
		seedCrossData(db, t)

		// 定义接受 PreparerContext 的通用函数
		prepareAndGet := func(p sqlex.PreparerContext, name string) (*CrossUser, error) {
			ctx := context.Background()
			stmt, err := sqlex.PreparexContext(ctx, p, "SELECT * FROM cross_users WHERE name = ?")
			if err != nil {
				return nil, err
			}
			defer stmt.Close()

			var user CrossUser
			err = stmt.Get(&user, name)
			return &user, err
		}

		// 通过 DB 调用
		t.Run("ViaDB", func(t *testing.T) {
			user, err := prepareAndGet(db, "Alice")
			if err != nil {
				t.Fatalf("[%s] prepareAndGet via DB failed: %v", dbLabel(db), err)
			}
			if user.Email != "alice@example.com" {
				t.Errorf("[%s] expected alice@example.com, got %s", dbLabel(db), user.Email)
			}
		})

		// 通过 Tx 调用
		t.Run("ViaTx", func(t *testing.T) {
			tx, err := db.Beginx()
			if err != nil {
				t.Fatalf("[%s] Beginx failed: %v", dbLabel(db), err)
			}
			defer tx.Rollback()

			user, err := prepareAndGet(tx, "Bob")
			if err != nil {
				t.Fatalf("[%s] prepareAndGet via Tx failed: %v", dbLabel(db), err)
			}
			if user.Email != "bob@example.com" {
				t.Errorf("[%s] expected bob@example.com, got %s", dbLabel(db), user.Email)
			}
		})

		// 通过 Conn 调用
		t.Run("ViaConn", func(t *testing.T) {
			ctx := context.Background()
			conn, err := db.Connx(ctx)
			if err != nil {
				t.Fatalf("[%s] Connx failed: %v", dbLabel(db), err)
			}
			defer conn.Close()

			user, err := prepareAndGet(conn, "Charlie")
			if err != nil {
				t.Fatalf("[%s] prepareAndGet via Conn failed: %v", dbLabel(db), err)
			}
			if user.Email != "charlie@example.com" {
				t.Errorf("[%s] expected charlie@example.com, got %s", dbLabel(db), user.Email)
			}
		})
	})
}

// ========================================================
// TestCrossDBNamedExtInterface — 验证 NamedExt 接口接受 DB 和 Tx
// ========================================================
func TestCrossDBNamedExtInterface(t *testing.T) {
	runWithSchema(crossSchema, t, func(db *sqlex.DB, t *testing.T, now string) {
		crossDBOnly(t)
		seedCrossData(db, t)

		// 定义接受 NamedExt 的通用函数
		getUserByName := func(ext sqlex.NamedExt, name string) (*CrossUser, error) {
			var user CrossUser
			err := ext.NamedGet(&user,
				`SELECT * FROM cross_users WHERE name = :name`,
				map[string]any{"name": name})
			return &user, err
		}

		// 通过 DB 调用
		user, err := getUserByName(db, "Alice")
		if err != nil {
			t.Fatalf("[%s] getUserByName via DB failed: %v", dbLabel(db), err)
		}
		if user.Email != "alice@example.com" {
			t.Errorf("[%s] expected alice@example.com via DB, got %s", dbLabel(db), user.Email)
		}

		// 通过 Tx 调用
		tx, err := db.Beginx()
		if err != nil {
			t.Fatalf("[%s] Beginx failed: %v", dbLabel(db), err)
		}
		defer tx.Rollback()

		user, err = getUserByName(tx, "Bob")
		if err != nil {
			t.Fatalf("[%s] getUserByName via Tx failed: %v", dbLabel(db), err)
		}
		if user.Email != "bob@example.com" {
			t.Errorf("[%s] expected bob@example.com via Tx, got %s", dbLabel(db), user.Email)
		}
	})
}

// ========================================================
// TestCrossDBBindExtInterface — 验证 BindExt 接口接受 DB 和 Tx
// ========================================================
func TestCrossDBBindExtInterface(t *testing.T) {
	runWithSchema(crossSchema, t, func(db *sqlex.DB, t *testing.T, now string) {
		crossDBOnly(t)
		seedCrossData(db, t)

		// 定义接受 BindExt 的通用函数
		listUsersByAge := func(ext sqlex.BindExt, minAge int) ([]CrossUser, error) {
			var users []CrossUser
			err := ext.Select(&users,
				"SELECT * FROM cross_users WHERE age > ? ORDER BY age", minAge)
			return users, err
		}

		// 通过 DB 调用
		users, err := listUsersByAge(db, 26)
		if err != nil {
			t.Fatalf("[%s] listUsersByAge via DB failed: %v", dbLabel(db), err)
		}
		if len(users) != 2 {
			t.Errorf("[%s] expected 2 users via DB, got %d", dbLabel(db), len(users))
		}

		// 通过 Tx 调用
		tx, err := db.Beginx()
		if err != nil {
			t.Fatalf("[%s] Beginx failed: %v", dbLabel(db), err)
		}
		defer tx.Rollback()

		users, err = listUsersByAge(tx, 26)
		if err != nil {
			t.Fatalf("[%s] listUsersByAge via Tx failed: %v", dbLabel(db), err)
		}
		if len(users) != 2 {
			t.Errorf("[%s] expected 2 users via Tx, got %d", dbLabel(db), len(users))
		}
	})
}

// ========================================================
// TestCrossDBBindExtAutoIN — 验证 BindExt.Select 自动 IN 展开
// ========================================================
func TestCrossDBBindExtAutoIN(t *testing.T) {
	runWithSchema(crossSchema, t, func(db *sqlex.DB, t *testing.T, now string) {
		crossDBOnly(t)
		seedCrossData(db, t)

		// 通过 BindExt 接口使用自动 IN
		listByNames := func(ext sqlex.BindExt, names []string) ([]CrossUser, error) {
			var users []CrossUser
			err := ext.Select(&users,
				"SELECT * FROM cross_users WHERE name IN (?) ORDER BY name", names)
			return users, err
		}

		users, err := listByNames(db, []string{"Alice", "Charlie"})
		if err != nil {
			t.Fatalf("[%s] BindExt.Select auto-IN via DB failed: %v", dbLabel(db), err)
		}
		if len(users) != 2 {
			t.Errorf("[%s] expected 2 users, got %d", dbLabel(db), len(users))
		}
	})
}
