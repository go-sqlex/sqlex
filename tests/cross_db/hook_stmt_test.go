// hook_stmt_test.go — P3-2: NamedStmt/Stmt Hook 测试
//
// 验证 Stmt 和 NamedStmt 在执行时正确触发 Hook 系统。
package cross_db_test

import (
	"context"
	"testing"

	sqlex "github.com/go-sqlex/sqlex"
)

// TestCrossDBHookStmt — 验证 Preparex 出来的 Stmt 触发 Hook
func TestCrossDBHookStmt(t *testing.T) {
	runWithSchema(crossSchema, t, func(db *sqlex.DB, t *testing.T, now string) {
		crossDBOnly(t)
		seedCrossData(db, t)

		dbCopy := sqlex.NewDb(db.DB, db.DriverName())
		hook := &crossTestHook{}
		dbCopy.AddHook(hook)

		ctx := context.Background()

		// Preparex 创建预编译语句
		stmt, err := dbCopy.PreparexContext(ctx, "SELECT * FROM cross_users WHERE name = ?")
		if err != nil {
			t.Fatalf("[%s] PreparexContext failed: %v", dbLabel(db), err)
		}
		defer stmt.Close()

		// 通过 Stmt.GetContext 执行
		var user CrossUser
		err = stmt.GetContext(ctx, &user, "Alice")
		if err != nil {
			t.Fatalf("[%s] Stmt.GetContext failed: %v", dbLabel(db), err)
		}

		if user.Name != "Alice" {
			t.Errorf("[%s] expected Alice, got %s", dbLabel(db), user.Name)
		}

		hook.mu.Lock()
		defer hook.mu.Unlock()

		// Stmt 应触发 Hook（至少 1 次 before + 1 次 after）
		if hook.beforeCount == 0 {
			t.Errorf("[%s] Stmt should trigger Hook, but BeforeQuery not called", dbLabel(db))
		}
		if hook.afterCount == 0 {
			t.Errorf("[%s] Stmt should trigger Hook, but AfterQuery not called", dbLabel(db))
		}
	})
}

// TestCrossDBHookNamedStmt — 验证 PrepareNamed 出来的 NamedStmt 触发 Hook
func TestCrossDBHookNamedStmt(t *testing.T) {
	runWithSchema(crossSchema, t, func(db *sqlex.DB, t *testing.T, now string) {
		crossDBOnly(t)
		seedCrossData(db, t)

		dbCopy := sqlex.NewDb(db.DB, db.DriverName())
		hook := &crossTestHook{}
		dbCopy.AddHook(hook)

		ctx := context.Background()

		// PrepareNamed 创建命名参数预编译语句
		nstmt, err := dbCopy.PrepareNamedContext(ctx, "SELECT * FROM cross_users WHERE name = :name")
		if err != nil {
			t.Fatalf("[%s] PrepareNamedContext failed: %v", dbLabel(db), err)
		}
		defer nstmt.Close()

		// 通过 NamedStmt.GetContext 执行
		var user CrossUser
		err = nstmt.GetContext(ctx, &user, map[string]any{"name": "Bob"})
		if err != nil {
			t.Fatalf("[%s] NamedStmt.GetContext failed: %v", dbLabel(db), err)
		}

		if user.Name != "Bob" {
			t.Errorf("[%s] expected Bob, got %s", dbLabel(db), user.Name)
		}

		hook.mu.Lock()
		defer hook.mu.Unlock()

		if hook.beforeCount == 0 {
			t.Errorf("[%s] NamedStmt should trigger Hook, but BeforeQuery not called", dbLabel(db))
		}
		if hook.afterCount == 0 {
			t.Errorf("[%s] NamedStmt should trigger Hook, but AfterQuery not called", dbLabel(db))
		}
	})
}

// TestCrossDBHookStmtExec — 验证 Stmt.MustExec 触发 Hook
func TestCrossDBHookStmtExec(t *testing.T) {
	runWithSchema(crossSchema, t, func(db *sqlex.DB, t *testing.T, now string) {
		crossDBOnly(t)
		seedCrossData(db, t)

		dbCopy := sqlex.NewDb(db.DB, db.DriverName())
		hook := &crossTestHook{}
		dbCopy.AddHook(hook)

		// Preparex 创建预编译语句
		stmt, err := dbCopy.Preparex("UPDATE cross_users SET age = ? WHERE name = ?")
		if err != nil {
			t.Fatalf("[%s] Preparex failed: %v", dbLabel(db), err)
		}
		defer stmt.Close()

		// 通过 Stmt.MustExec 执行
		stmt.MustExec(99, "Alice")

		hook.mu.Lock()
		defer hook.mu.Unlock()

		if hook.beforeCount == 0 {
			t.Errorf("[%s] Stmt.MustExec should trigger Hook, but BeforeQuery not called", dbLabel(db))
		}
		if hook.afterCount == 0 {
			t.Errorf("[%s] Stmt.MustExec should trigger Hook, but AfterQuery not called", dbLabel(db))
		}

		// Hook 事件应包含查询字符串
		if len(hook.queries) > 0 && hook.queries[0] == "" {
			t.Errorf("[%s] Hook query should not be empty for Stmt", dbLabel(db))
		}
	})
}

// TestCrossDBHookStmtInTx — 验证事务中的 Stmt 也触发 Hook
func TestCrossDBHookStmtInTx(t *testing.T) {
	runWithSchema(crossSchema, t, func(db *sqlex.DB, t *testing.T, now string) {
		crossDBOnly(t)
		seedCrossData(db, t)

		dbCopy := sqlex.NewDb(db.DB, db.DriverName())
		hook := &crossTestHook{}
		dbCopy.AddHook(hook)

		ctx := context.Background()

		tx, err := dbCopy.BeginTxx(ctx, nil)
		if err != nil {
			t.Fatalf("[%s] BeginTxx failed: %v", dbLabel(db), err)
		}
		defer tx.Rollback()

		// 在事务中创建预编译语句
		stmt, err := tx.PreparexContext(ctx, "SELECT * FROM cross_users WHERE name = ?")
		if err != nil {
			t.Fatalf("[%s] Tx.PreparexContext failed: %v", dbLabel(db), err)
		}
		defer stmt.Close()

		var user CrossUser
		err = stmt.GetContext(ctx, &user, "Charlie")
		if err != nil {
			t.Fatalf("[%s] Stmt.GetContext in Tx failed: %v", dbLabel(db), err)
		}

		hook.mu.Lock()
		defer hook.mu.Unlock()

		if hook.beforeCount == 0 {
			t.Errorf("[%s] Stmt in Tx should trigger Hook, but BeforeQuery not called", dbLabel(db))
		}
	})
}
