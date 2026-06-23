// hook_test.go — cross-database integration tests for Hook aspects
package cross_db_test

import (
	"context"
	"sync"
	"testing"
	"time"

	sqlex "github.com/go-sqlex/sqlex"
)

// ========================================================
// TestCrossDBHookBasic — 验证 Hook 基础触发
// ========================================================
func TestCrossDBHookBasic(t *testing.T) {
	runWithSchema(crossSchema, t, func(db *sqlex.DB, t *testing.T, now string) {
		crossDBOnly(t)
		seedCrossData(db, t)

		// 创建独立 DB 包装避免污染全局 hooks
		dbCopy := sqlex.NewDB(db.DB, db.DriverName())
		hook := &crossTestHook{}
		dbCopy.AddHook(hook)

		ctx := context.Background()

		// ExecContext 触发 Hook
		_, err := dbCopy.ExecContext(ctx, "UPDATE cross_users SET age = ? WHERE name = ?", 31, "Alice")
		if err != nil {
			t.Fatalf("[%s] ExecContext failed: %v", dbLabel(db), err)
		}

		// SelectContext 触发 Hook
		var users []CrossUser
		err = dbCopy.SelectContext(ctx, &users, "SELECT * FROM cross_users ORDER BY id")
		if err != nil {
			t.Fatalf("[%s] SelectContext failed: %v", dbLabel(db), err)
		}

		// GetContext 触发 Hook
		var user CrossUser
		err = dbCopy.GetContext(ctx, &user, "SELECT * FROM cross_users WHERE name = ?", "Alice")
		if err != nil {
			t.Fatalf("[%s] GetContext failed: %v", dbLabel(db), err)
		}

		hook.mu.Lock()
		defer hook.mu.Unlock()

		if hook.beforeCount != 3 {
			t.Errorf("[%s] expected 3 BeforeQuery calls, got %d", dbLabel(db), hook.beforeCount)
		}
		if hook.afterCount != 3 {
			t.Errorf("[%s] expected 3 AfterQuery calls, got %d", dbLabel(db), hook.afterCount)
		}
	})
}

// ========================================================
// TestCrossDBHookOnion — 验证多 Hook 的洋葱模型执行顺序
// ========================================================
func TestCrossDBHookOnion(t *testing.T) {
	runWithSchema(crossSchema, t, func(db *sqlex.DB, t *testing.T, now string) {
		crossDBOnly(t)
		seedCrossData(db, t)

		dbCopy := sqlex.NewDB(db.DB, db.DriverName())
		var order []string
		var mu sync.Mutex

		// 注册 3 个 Hook
		dbCopy.AddHook(&crossOrderHook{name: "A", order: &order, mu: &mu})
		dbCopy.AddHook(&crossOrderHook{name: "B", order: &order, mu: &mu})
		dbCopy.AddHook(&crossOrderHook{name: "C", order: &order, mu: &mu})

		ctx := context.Background()
		var user CrossUser
		err := dbCopy.GetContext(ctx, &user, "SELECT * FROM cross_users WHERE name = ?", "Alice")
		if err != nil {
			t.Fatalf("[%s] GetContext failed: %v", dbLabel(db), err)
		}

		mu.Lock()
		defer mu.Unlock()

		// 期望顺序：before:A, before:B, before:C, after:C, after:B, after:A
		expected := []string{"before:A", "before:B", "before:C", "after:C", "after:B", "after:A"}
		if len(order) != len(expected) {
			t.Fatalf("[%s] expected %d hook calls, got %d: %v", dbLabel(db), len(expected), len(order), order)
		}
		for i, exp := range expected {
			if order[i] != exp {
				t.Errorf("[%s] hook call %d: expected %s, got %s", dbLabel(db), i, exp, order[i])
			}
		}
	})
}

// ========================================================
// TestCrossDBHookInheritTx — 验证 Tx 自动继承 DB 的 Hook
// ========================================================
func TestCrossDBHookInheritTx(t *testing.T) {
	runWithSchema(crossSchema, t, func(db *sqlex.DB, t *testing.T, now string) {
		crossDBOnly(t)
		seedCrossData(db, t)

		dbCopy := sqlex.NewDB(db.DB, db.DriverName())
		hook := &crossTestHook{}
		dbCopy.AddHook(hook)

		ctx := context.Background()
		tx, err := dbCopy.BeginTxx(ctx, nil)
		if err != nil {
			t.Fatalf("[%s] BeginTxx failed: %v", dbLabel(db), err)
		}
		defer tx.Rollback()

		// 事务中的查询应触发 Hook
		var user CrossUser
		err = tx.GetContext(ctx, &user, "SELECT * FROM cross_users WHERE name = ?", "Alice")
		if err != nil {
			t.Fatalf("[%s] Tx.GetContext failed: %v", dbLabel(db), err)
		}

		hook.mu.Lock()
		defer hook.mu.Unlock()

		if hook.beforeCount == 0 {
			t.Errorf("[%s] Tx should inherit DB hooks, but BeforeQuery not called", dbLabel(db))
		}
		if hook.afterCount == 0 {
			t.Errorf("[%s] Tx should inherit DB hooks, but AfterQuery not called", dbLabel(db))
		}
	})
}

// ========================================================
// TestCrossDBHookInheritConn — 验证 Conn 自动继承 DB 的 Hook
// ========================================================
func TestCrossDBHookInheritConn(t *testing.T) {
	runWithSchema(crossSchema, t, func(db *sqlex.DB, t *testing.T, now string) {
		crossDBOnly(t)
		seedCrossData(db, t)

		dbCopy := sqlex.NewDB(db.DB, db.DriverName())
		hook := &crossTestHook{}
		dbCopy.AddHook(hook)

		ctx := context.Background()
		conn, err := dbCopy.Connx(ctx)
		if err != nil {
			t.Fatalf("[%s] Connx failed: %v", dbLabel(db), err)
		}
		defer conn.Close()

		var user CrossUser
		err = conn.GetContext(ctx, &user, "SELECT * FROM cross_users WHERE name = ?", "Alice")
		if err != nil {
			t.Fatalf("[%s] Conn.GetContext failed: %v", dbLabel(db), err)
		}

		hook.mu.Lock()
		defer hook.mu.Unlock()

		if hook.beforeCount == 0 {
			t.Errorf("[%s] Conn should inherit DB hooks, but BeforeQuery not called", dbLabel(db))
		}
		if hook.afterCount == 0 {
			t.Errorf("[%s] Conn should inherit DB hooks, but AfterQuery not called", dbLabel(db))
		}
	})
}

// ========================================================
// TestCrossDBHookDuration — 验证 QueryEvent.Duration 包含合理耗时
// ========================================================
func TestCrossDBHookDuration(t *testing.T) {
	runWithSchema(crossSchema, t, func(db *sqlex.DB, t *testing.T, now string) {
		crossDBOnly(t)
		seedCrossData(db, t)

		dbCopy := sqlex.NewDB(db.DB, db.DriverName())
		hook := &crossTestHook{}
		dbCopy.AddHook(hook)

		ctx := context.Background()
		var users []CrossUser
		err := dbCopy.SelectContext(ctx, &users, "SELECT * FROM cross_users")
		if err != nil {
			t.Fatalf("[%s] SelectContext failed: %v", dbLabel(db), err)
		}

		hook.mu.Lock()
		defer hook.mu.Unlock()

		if len(hook.durations) == 0 {
			t.Fatalf("[%s] no durations recorded", dbLabel(db))
		}
		d := hook.durations[0]
		if d <= 0 {
			t.Errorf("[%s] expected positive duration, got %v", dbLabel(db), d)
		}
		// 一般数据库查询应在合理时间内完成
		if d > 30*time.Second {
			t.Errorf("[%s] duration seems too long: %v", dbLabel(db), d)
		}
	})
}

// ========================================================
// TestCrossDBHookError — 验证 SQL 出错时 QueryEvent.Error 包含错误
// ========================================================
func TestCrossDBHookError(t *testing.T) {
	runWithSchema(crossSchema, t, func(db *sqlex.DB, t *testing.T, now string) {
		crossDBOnly(t)

		dbCopy := sqlex.NewDB(db.DB, db.DriverName())
		hook := &crossTestHook{}
		dbCopy.AddHook(hook)

		ctx := context.Background()
		// 执行无效 SQL
		_, err := dbCopy.ExecContext(ctx, "THIS IS INVALID SQL")
		if err == nil {
			t.Fatalf("[%s] expected error from invalid SQL", dbLabel(db))
		}

		hook.mu.Lock()
		defer hook.mu.Unlock()

		// AfterQuery 应记录错误
		if len(hook.errors) == 0 {
			t.Fatalf("[%s] no errors recorded in hook", dbLabel(db))
		}
		if hook.errors[0] == nil {
			t.Errorf("[%s] expected non-nil error in hook, got nil", dbLabel(db))
		}
	})
}

// ========================================================
// TestCrossDBNoHook — 验证无 Hook 时查询正常工作（基线验证）
// ========================================================
func TestCrossDBNoHook(t *testing.T) {
	runWithSchema(crossSchema, t, func(db *sqlex.DB, t *testing.T, now string) {
		crossDBOnly(t)
		seedCrossData(db, t)

		// 使用不带 Hook 的 DB
		dbCopy := sqlex.NewDB(db.DB, db.DriverName())
		// 不注册任何 Hook

		ctx := context.Background()
		var users []CrossUser
		err := dbCopy.SelectContext(ctx, &users, "SELECT * FROM cross_users ORDER BY id")
		if err != nil {
			t.Fatalf("[%s] SelectContext without hooks failed: %v", dbLabel(db), err)
		}
		if len(users) != 3 {
			t.Errorf("[%s] expected 3 users, got %d", dbLabel(db), len(users))
		}
	})
}
