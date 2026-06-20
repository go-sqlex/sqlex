// integration_test.go — sqlex general integration tests (black-box)
//
// Coverage:
//  1. Standard SQL CRUD (Select / Get / Exec)
//  2. Bug fix verification (Rebind escape, Unicode named queries, :: handling,
//     NamedStmt.Exec return value, ConnectContext leak fix)
//  3. New features (NamedGet / NamedSelect with IN expansion / NamedExecContext /
//     CloseWithErr / Hook / auto IN expansion / NamedExt + BindExt unified interfaces)
//
// Run:
//
//	go test -v -count=1 -timeout=120s ./tests/integration/
package integration_test

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	sqlex "github.com/go-sqlex/sqlex"
	"github.com/go-sqlex/sqlex/types"
)

// ---------- 辅助 ----------

// 集成测试专用 schema
var integrationSchema = Schema{
	Create: `
CREATE TABLE int_users (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	name TEXT NOT NULL,
	email TEXT NOT NULL,
	age INTEGER DEFAULT 0,
	created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE int_orders (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	user_id INTEGER NOT NULL,
	amount REAL NOT NULL,
	status VARCHAR(50) DEFAULT 'pending'
);
`,
	Drop: `
DROP TABLE IF EXISTS int_users;
DROP TABLE IF EXISTS int_orders;
`,
}

type IntUser struct {
	ID        int       `db:"id"`
	Name      string    `db:"name"`
	Email     string    `db:"email"`
	Age       int       `db:"age"`
	CreatedAt time.Time `db:"created_at"`
}

type IntOrder struct {
	ID     int     `db:"id"`
	UserID int     `db:"user_id"`
	Amount float64 `db:"amount"`
	Status string  `db:"status"`
}

// seedIntegrationData 插入测试种子数据
func seedIntegrationData(db *sqlex.DB, t *testing.T) {
	t.Helper()
	tx := db.MustBegin()
	tx.MustExec(`INSERT INTO int_users (name, email, age) VALUES ('Alice', 'alice@example.com', 30)`)
	tx.MustExec(`INSERT INTO int_users (name, email, age) VALUES ('Bob', 'bob@example.com', 25)`)
	tx.MustExec(`INSERT INTO int_users (name, email, age) VALUES ('Charlie', 'charlie@example.com', 35)`)
	tx.MustExec(`INSERT INTO int_orders (user_id, amount, status) VALUES (1, 99.9, 'paid')`)
	tx.MustExec(`INSERT INTO int_orders (user_id, amount, status) VALUES (1, 50.0, 'pending')`)
	tx.MustExec(`INSERT INTO int_orders (user_id, amount, status) VALUES (2, 200.0, 'paid')`)
	tx.Commit()
}

// ========================================================
// 1. 常规 SQL CRUD
// ========================================================

func TestIntegrationBasicCRUD(t *testing.T) {
	runWithSchema(integrationSchema, t, func(db *sqlex.DB, t *testing.T, now string) {
		seedIntegrationData(db, t)

		// --- Select: 查询多行 ---
		var users []IntUser
		err := db.Select(&users, "SELECT * FROM int_users ORDER BY id")
		if err != nil {
			t.Fatalf("Select failed: %v", err)
		}
		if len(users) != 3 {
			t.Fatalf("expected 3 users, got %d", len(users))
		}
		if users[0].Name != "Alice" || users[1].Name != "Bob" || users[2].Name != "Charlie" {
			t.Errorf("unexpected user names: %v", users)
		}

		// --- Get: 查询单行（统一使用 ? 占位符，框架自动 Rebind）---
		var user IntUser
		err = db.Get(&user, "SELECT * FROM int_users WHERE name = ?", "Alice")
		if err != nil {
			t.Fatalf("Get failed: %v", err)
		}
		if user.Email != "alice@example.com" || user.Age != 30 {
			t.Errorf("unexpected user: %+v", user)
		}

		// --- Get: 无结果应返回 sql.ErrNoRows ---
		err = db.Get(&user, "SELECT * FROM int_users WHERE name = ?", "NonExistent")
		if !errors.Is(err, sql.ErrNoRows) {
			t.Errorf("expected sql.ErrNoRows, got %v", err)
		}

		// --- Exec: 更新 ---
		result, err := db.Exec("UPDATE int_users SET age = ? WHERE name = ?", 31, "Alice")
		if err != nil {
			t.Fatalf("Exec UPDATE failed: %v", err)
		}
		rowsAffected, _ := result.RowsAffected()
		if rowsAffected != 1 {
			t.Errorf("expected 1 row affected, got %d", rowsAffected)
		}

		// 验证更新生效
		err = db.Get(&user, "SELECT * FROM int_users WHERE name = ?", "Alice")
		if err != nil {
			t.Fatalf("Get after UPDATE failed: %v", err)
		}
		if user.Age != 31 {
			t.Errorf("expected age 31, got %d", user.Age)
		}

		// --- Exec: 删除 ---
		result, err = db.Exec("DELETE FROM int_users WHERE name = ?", "Charlie")
		if err != nil {
			t.Fatalf("Exec DELETE failed: %v", err)
		}
		rowsAffected, _ = result.RowsAffected()
		if rowsAffected != 1 {
			t.Errorf("expected 1 row deleted, got %d", rowsAffected)
		}

		// 验证剩余2条
		var count int
		err = db.Get(&count, "SELECT COUNT(*) FROM int_users")
		if err != nil {
			t.Fatalf("Get COUNT failed: %v", err)
		}
		if count != 2 {
			t.Errorf("expected 2 users, got %d", count)
		}

		// --- Exec: 插入 ---
		result, err = db.Exec("INSERT INTO int_users (name, email, age) VALUES (?, ?, ?)", "Dave", "dave@example.com", 28)
		if err != nil {
			t.Fatalf("Exec INSERT failed: %v", err)
		}
		rowsAffected, _ = result.RowsAffected()
		if rowsAffected != 1 {
			t.Errorf("expected 1 row inserted, got %d", rowsAffected)
		}
		// 注意: PostgreSQL 和 SQL Server 不支持 LastInsertId
		if db.DriverName() != "postgres" && db.DriverName() != "sqlserver" {
			lastID, _ := result.LastInsertId()
			if lastID <= 0 {
				t.Errorf("expected positive LastInsertId, got %d", lastID)
			}
		}
	})
}

// ========================================================
// 2. Bug Fix 验证
// ========================================================

// 2.1 Rebind 转义问号: \? → ? 和 ?? → ?
func TestIntegrationRebindEscapeQuestionMark(t *testing.T) {
	// \? 转义（在字符串字面量外部）
	result := sqlex.Rebind(sqlex.DOLLAR, `SELECT * FROM foo WHERE bar = ? AND name = '\?'`)
	// 字符串字面量外的 ? 被替换为 $1，字面量内的 \? 保持原样
	expected := `SELECT * FROM foo WHERE bar = $1 AND name = '\?'`
	if result != expected {
		t.Errorf("Rebind \\? escape failed:\n  expected: %s\n  got:      %s", expected, result)
	}

	// 字符串字面量内的 ? 自动被跳过，无需转义
	result = sqlex.Rebind(sqlex.DOLLAR, `SELECT * FROM foo WHERE bar = ? AND data LIKE '?%'`)
	expected = `SELECT * FROM foo WHERE bar = $1 AND data LIKE '?%'`
	if result != expected {
		t.Errorf("Rebind string literal ? skip failed:\n  expected: %s\n  got:      %s", expected, result)
	}

	// ?? 转义（在字符串字面量外部）
	result = sqlex.Rebind(sqlex.DOLLAR, `SELECT * FROM foo WHERE a = ? AND b ?? 'key'`)
	expected = `SELECT * FROM foo WHERE a = $1 AND b ? 'key'`
	if result != expected {
		t.Errorf("Rebind ?? escape failed:\n  expected: %s\n  got:      %s", expected, result)
	}

	// 多参数混合转义：字符串字面量中的 ? 被跳过，外部的 ?? 正常转义
	result = sqlex.Rebind(sqlex.DOLLAR, `SELECT * FROM t WHERE a = ? AND b ?? 'k' AND c = ? AND d LIKE '?x'`)
	expected = `SELECT * FROM t WHERE a = $1 AND b ? 'k' AND c = $2 AND d LIKE '?x'`
	if result != expected {
		t.Errorf("Rebind mixed escape failed:\n  expected: %s\n  got:      %s", expected, result)
	}
}

// 2.3 命名查询 Unicode — 数据库级别
// 注：compileNamedQuery 的纯单元测试已在 named_test.go 中覆盖（TestCompileQuery），此处仅保留数据库级别验证
// TestIntegrationNamedQueryUnicodeDB 验证 Unicode 数据值的端到端读写。
//
// 注意：sqlex 不支持 Unicode 命名参数名（:名前 这类），但 Unicode 作为参数值
// 或表数据完全正常。本测试用 ASCII 参数名 + Unicode 值验证真实场景。
func TestIntegrationNamedQueryUnicodeDB(t *testing.T) {
	runWithSchema(integrationSchema, t, func(db *sqlex.DB, t *testing.T, now string) {
		// ASCII 参数名 + Unicode 数据值
		_, err := db.NamedExec(
			`INSERT INTO int_users (name, email, age) VALUES (:name, :email, :age)`,
			map[string]any{"name": "太郎", "email": "taro@example.com", "age": 28},
		)
		if err != nil {
			t.Fatalf("NamedExec with Unicode values failed: %v", err)
		}

		var user IntUser
		err = db.Get(&user, "SELECT * FROM int_users WHERE name = ?", "太郎")
		if err != nil {
			t.Fatalf("Get Unicode user failed: %v", err)
		}
		if user.Email != "taro@example.com" || user.Age != 28 {
			t.Errorf("unexpected user: %+v", user)
		}
	})
}

// 2.4 NamedStmt.Exec 返回值修正
// 注：compileNamedQuery 双冒号处理的纯单元测试已在 named_test.go 中覆盖（TestCompileQuery）
func TestIntegrationNamedStmtExecResult(t *testing.T) {
	runWithSchema(integrationSchema, t, func(db *sqlex.DB, t *testing.T, now string) {
		ns, err := db.PrepareNamed(`INSERT INTO int_users (name, email, age) VALUES (:name, :email, :age)`)
		if err != nil {
			t.Fatalf("PrepareNamed failed: %v", err)
		}
		defer ns.Close()

		result, err := ns.Exec(map[string]any{"name": "StmtTest", "email": "stmt@test.com", "age": 40})
		if err != nil {
			t.Fatalf("NamedStmt.Exec failed: %v", err)
		}

		// 验证 sql.Result 非 nil
		if result == nil {
			t.Fatal("expected non-nil sql.Result from NamedStmt.Exec")
		}
		// 注意: PostgreSQL 和 SQL Server 不支持 LastInsertId
		if db.DriverName() != "postgres" && db.DriverName() != "sqlserver" {
			lastID, err := result.LastInsertId()
			if err != nil {
				t.Fatalf("LastInsertId failed: %v", err)
			}
			if lastID <= 0 {
				t.Errorf("expected positive LastInsertId, got %d", lastID)
			}
		}

		rowsAff, err := result.RowsAffected()
		if err != nil {
			t.Fatalf("RowsAffected failed: %v", err)
		}
		if rowsAff != 1 {
			t.Errorf("expected 1 row affected, got %d", rowsAff)
		}
	})
}

// 2.6 ConnectContext 泄漏修复: Ping 失败时 db.Close() 被调用
func TestIntegrationConnectContextPingFailure(t *testing.T) {
	// 连接一个不存在的数据源应该返回错误，而不是泄漏连接
	// 使用一个无效的 DSN 来触发连接错误
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	_, err := sqlex.ConnectContext(ctx, "sqlite3", "file:/nonexistent/path?mode=ro")
	if err == nil {
		t.Error("expected error from ConnectContext with invalid DSN, got nil")
	}
	// 如果没有 panic 或挂起，说明连接资源被正确清理
}

// ========================================================
// 3. 新增功能测试
// ========================================================

// 3.1 NamedGet / NamedSelect
func TestIntegrationNamedGetSelect(t *testing.T) {
	runWithSchema(integrationSchema, t, func(db *sqlex.DB, t *testing.T, now string) {
		seedIntegrationData(db, t)

		// NamedGet — 命名参数查询单行
		var user IntUser
		err := db.NamedGet(&user, `SELECT * FROM int_users WHERE name = :name`,
			map[string]any{"name": "Alice"})
		if err != nil {
			t.Fatalf("NamedGet failed: %v", err)
		}
		if user.Email != "alice@example.com" {
			t.Errorf("expected alice@example.com, got %s", user.Email)
		}

		// NamedGet — 无结果
		err = db.NamedGet(&user, `SELECT * FROM int_users WHERE name = :name`,
			map[string]any{"name": "Nobody"})
		if !errors.Is(err, sql.ErrNoRows) {
			t.Errorf("expected sql.ErrNoRows, got %v", err)
		}

		// NamedSelect — 命名参数查询多行
		var users []IntUser
		err = db.NamedSelect(&users, `SELECT * FROM int_users WHERE age > :min_age ORDER BY age`,
			map[string]any{"min_age": 26})
		if err != nil {
			t.Fatalf("NamedSelect failed: %v", err)
		}
		if len(users) != 2 {
			t.Fatalf("expected 2 users (Alice=30, Charlie=35), got %d", len(users))
		}
		if users[0].Name != "Alice" || users[1].Name != "Charlie" {
			t.Errorf("unexpected order: %v, %v", users[0].Name, users[1].Name)
		}

		// NamedSelect — 使用结构体作为参数
		type AgeFilter struct {
			MinAge int `db:"min_age"`
		}
		var filtered []IntUser
		err = db.NamedSelect(&filtered, `SELECT * FROM int_users WHERE age > :min_age`,
			AgeFilter{MinAge: 30})
		if err != nil {
			t.Fatalf("NamedSelect with struct param failed: %v", err)
		}
		if len(filtered) != 1 || filtered[0].Name != "Charlie" {
			t.Errorf("expected [Charlie], got %v", filtered)
		}
	})
}

// 3.2 NamedSelect + IN 展开 / NamedExecContext + IN 展开
func TestIntegrationNamedSelectWithIN(t *testing.T) {
	runWithSchema(integrationSchema, t, func(db *sqlex.DB, t *testing.T, now string) {
		seedIntegrationData(db, t)

		// NamedSelect — 命名参数 + IN 展开（已统一，无需 NamedInSelect）
		var users []IntUser
		err := db.NamedSelect(&users,
			`SELECT * FROM int_users WHERE name IN (:names) ORDER BY name`,
			map[string]any{"names": []string{"Alice", "Charlie"}})
		if err != nil {
			t.Fatalf("NamedSelect with IN failed: %v", err)
		}
		if len(users) != 2 {
			t.Fatalf("expected 2 users, got %d", len(users))
		}
		if users[0].Name != "Alice" || users[1].Name != "Charlie" {
			t.Errorf("unexpected users: %v, %v", users[0].Name, users[1].Name)
		}

		// NamedExecContext —— 命名参数 + IN 执行
		result, err := db.NamedExecContext(context.Background(),
			`DELETE FROM int_orders WHERE user_id IN (:user_ids)`,
			map[string]any{"user_ids": []int{1, 2}})
		if err != nil {
			t.Fatalf("NamedExecContext failed: %v", err)
		}
		rowsAff, _ := result.RowsAffected()
		if rowsAff != 3 {
			t.Errorf("expected 3 orders deleted, got %d", rowsAff)
		}

		// 验证订单已清空
		var count int
		db.Get(&count, "SELECT COUNT(*) FROM int_orders")
		if count != 0 {
			t.Errorf("expected 0 orders, got %d", count)
		}
	})
}

// 3.3 Select/Get 自动 IN 展开
func TestIntegrationAutoINExpansion(t *testing.T) {
	runWithSchema(integrationSchema, t, func(db *sqlex.DB, t *testing.T, now string) {
		seedIntegrationData(db, t)

		// Select 自动检测切片参数展开 IN 子句
		var users []IntUser
		ids := []int{1, 3}
		err := db.Select(&users, "SELECT * FROM int_users WHERE id IN (?) ORDER BY id", ids)
		if err != nil {
			t.Fatalf("Select auto-IN failed: %v", err)
		}
		if len(users) != 2 {
			t.Fatalf("expected 2 users, got %d", len(users))
		}
		if users[0].Name != "Alice" || users[1].Name != "Charlie" {
			t.Errorf("unexpected users: %v, %v", users[0].Name, users[1].Name)
		}

		// Get 自动 IN（虽然不常见，但应该不报错）
		var user IntUser
		err = db.Get(&user, selectTop1(db, "SELECT * FROM int_users WHERE id IN (?) LIMIT 1"), []int{2})
		if err != nil {
			t.Fatalf("Get auto-IN failed: %v", err)
		}
		if user.Name != "Bob" {
			t.Errorf("expected Bob, got %s", user.Name)
		}
	})
}

// 3.4 CloseWithErr 自动事务管理
func TestIntegrationCloseWithErr(t *testing.T) {
	runWithSchema(integrationSchema, t, func(db *sqlex.DB, t *testing.T, now string) {

		// 场景一：err == nil → 应自动 Commit
		func() {
			var err error
			tx, txErr := db.Beginx()
			if txErr != nil {
				t.Fatalf("Beginx failed: %v", txErr)
			}
			defer func() { tx.CloseWithErr(err) }()

			_, err = tx.Exec("INSERT INTO int_users (name, email, age) VALUES (?, ?, ?)",
				"CommitUser", "commit@test.com", 20)
			if err != nil {
				t.Fatalf("tx.Exec failed: %v", err)
			}
			// err 为 nil，CloseWithErr 应自动 Commit
		}()

		// 验证数据已提交
		var user IntUser
		err := db.Get(&user, "SELECT * FROM int_users WHERE name = ?", "CommitUser")
		if err != nil {
			t.Fatalf("expected CommitUser to be committed, got error: %v", err)
		}

		// 场景二：err != nil → 应自动 Rollback
		func() {
			var err error
			tx, txErr := db.Beginx()
			if txErr != nil {
				t.Fatalf("Beginx failed: %v", txErr)
			}
			defer func() { tx.CloseWithErr(err) }()

			_, err = tx.Exec("INSERT INTO int_users (name, email, age) VALUES (?, ?, ?)",
				"RollbackUser", "rollback@test.com", 20)
			if err != nil {
				t.Fatalf("tx.Exec failed: %v", err)
			}
			// 模拟业务错误
			err = fmt.Errorf("business error")
			// CloseWithErr 检测到 err != nil，应自动 Rollback
		}()

		// 验证数据未提交
		err = db.Get(&user, "SELECT * FROM int_users WHERE name = ?", "RollbackUser")
		if !errors.Is(err, sql.ErrNoRows) {
			t.Errorf("expected RollbackUser to be rolled back, got: %v", err)
		}
	})
}

// 3.5 Hook 机制
// 注意: Hook 只在 Context 版本的方法中触发（QueryxContext/QueryRowxContext/ExecContext）
func TestIntegrationHook(t *testing.T) {
	runWithSchema(integrationSchema, t, func(db *sqlex.DB, t *testing.T, now string) {
		// 创建一个记录查询的 Hook
		hook := &testHook{}
		db.AddHook(hook)

		seedIntegrationData(db, t)

		// 使用 Context 版本的查询方法（Hook 仅在 Context 路径触发）
		ctx := context.Background()
		var users []IntUser
		err := db.SelectContext(ctx, &users, "SELECT * FROM int_users ORDER BY id")
		if err != nil {
			t.Fatalf("SelectContext failed: %v", err)
		}

		// 验证 Hook 被调用
		hook.mu.Lock()
		defer hook.mu.Unlock()

		if hook.beforeCount == 0 {
			t.Error("expected BeforeQuery to be called at least once")
		}
		if hook.afterCount == 0 {
			t.Error("expected AfterQuery to be called at least once")
		}
		// 验证最近的查询包含 SELECT
		found := false
		for _, q := range hook.queries {
			if q == "SELECT * FROM int_users ORDER BY id" {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected to find SELECT query in hook records, got: %v", hook.queries)
		}
	})
}

// testHook 是测试用的 Hook 实现
type testHook struct {
	mu          sync.Mutex
	beforeCount int
	afterCount  int
	queries     []string
}

func (h *testHook) BeforeQuery(ctx context.Context, event *sqlex.QueryEvent) context.Context {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.beforeCount++
	h.queries = append(h.queries, event.Query)
	return ctx
}

func (h *testHook) AfterQuery(ctx context.Context, event *sqlex.QueryEvent) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.afterCount++
}

// 3.5b Hook 洋葱模型 — 多 Hook 按正序/反序执行
func TestIntegrationHookOnionModel(t *testing.T) {
	runWithSchema(integrationSchema, t, func(db *sqlex.DB, t *testing.T, now string) {
		var order []string
		var mu sync.Mutex

		hook1 := &orderHook{name: "A", order: &order, mu: &mu}
		hook2 := &orderHook{name: "B", order: &order, mu: &mu}

		db.AddHook(hook1)
		db.AddHook(hook2)

		// 使用 Context 版本触发 Hook（Hook 仅在查询路径 QueryxContext/QueryRowxContext 触发）
		ctx := context.Background()
		_, err := db.ExecContext(ctx, "INSERT INTO int_users (name, email, age) VALUES (?, ?, ?)", "Test", "test@test.com", 1)
		if err != nil {
			t.Fatalf("ExecContext failed: %v", err)
		}
		var dummy IntUser
		db.GetContext(ctx, &dummy, "SELECT * FROM int_users WHERE name = ?", "Test")

		mu.Lock()
		defer mu.Unlock()

		// 期望: BeforeA → BeforeB → AfterB → AfterA
		expected := []string{"before:A", "before:B", "after:B", "after:A"}
		if len(order) < 4 {
			t.Fatalf("expected at least 4 hook calls, got %d: %v", len(order), order)
		}
		// 取最后4个（因为可能有建表等操作的 hook 调用）
		last4 := order[len(order)-4:]
		for i, exp := range expected {
			if last4[i] != exp {
				t.Errorf("hook order[%d]: expected %s, got %s (full: %v)", i, exp, last4[i], order)
			}
		}
	})
}

type orderHook struct {
	name  string
	order *[]string
	mu    *sync.Mutex
}

func (h *orderHook) BeforeQuery(ctx context.Context, event *sqlex.QueryEvent) context.Context {
	h.mu.Lock()
	defer h.mu.Unlock()
	*h.order = append(*h.order, "before:"+h.name)
	return ctx
}

func (h *orderHook) AfterQuery(ctx context.Context, event *sqlex.QueryEvent) {
	h.mu.Lock()
	defer h.mu.Unlock()
	*h.order = append(*h.order, "after:"+h.name)
}

// 3.5c Hook 继承到事务
func TestIntegrationHookInheritedByTx(t *testing.T) {
	runWithSchema(integrationSchema, t, func(db *sqlex.DB, t *testing.T, now string) {
		hook := &testHook{}
		db.AddHook(hook)

		tx, err := db.Beginx()
		if err != nil {
			t.Fatalf("Beginx failed: %v", err)
		}
		defer tx.Rollback()

		// 使用 Context 查询路径触发 Hook
		ctx := context.Background()
		_, _ = tx.ExecContext(ctx, "INSERT INTO int_users (name, email, age) VALUES ('TxHook', 'txhook@test.com', 10)")
		var dummy IntUser
		tx.GetContext(ctx, &dummy, "SELECT * FROM int_users WHERE name = ?", "TxHook")

		hook.mu.Lock()
		defer hook.mu.Unlock()

		// 事务中的查询操作也应触发 Hook
		found := false
		// Hook 记录的是 Rebind 后的查询格式（PG 为 $1，MySQL/SQLite 为 ?）
		expectedQuery := db.Rebind("SELECT * FROM int_users WHERE name = ?")
		for _, q := range hook.queries {
			if q == expectedQuery {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected tx query to trigger hook, got queries: %v", hook.queries)
		}
	})
}

// 3.6 NamedExt 统一接口
func TestIntegrationNamedExtInterface(t *testing.T) {
	runWithSchema(integrationSchema, t, func(db *sqlex.DB, t *testing.T, now string) {
		seedIntegrationData(db, t)

		// 定义一个接受 NamedExt 的通用函数
		getUserByName := func(ext sqlex.NamedExt, name string) (*IntUser, error) {
			var user IntUser
			err := ext.NamedGet(&user, `SELECT * FROM int_users WHERE name = :name`,
				map[string]any{"name": name})
			if err != nil {
				return nil, err
			}
			return &user, nil
		}

		// 通过 DB 调用
		user, err := getUserByName(db, "Alice")
		if err != nil {
			t.Fatalf("getUserByName via DB failed: %v", err)
		}
		if user.Email != "alice@example.com" {
			t.Errorf("expected alice@example.com, got %s", user.Email)
		}

		// 通过 Tx 调用
		tx, err := db.Beginx()
		if err != nil {
			t.Fatalf("Beginx failed: %v", err)
		}
		defer tx.Rollback()

		user, err = getUserByName(tx, "Bob")
		if err != nil {
			t.Fatalf("getUserByName via Tx failed: %v", err)
		}
		if user.Email != "bob@example.com" {
			t.Errorf("expected bob@example.com, got %s", user.Email)
		}
	})
}

// 3.6b BindExt 统一接口
func TestIntegrationBindExtInterface(t *testing.T) {
	runWithSchema(integrationSchema, t, func(db *sqlex.DB, t *testing.T, now string) {
		seedIntegrationData(db, t)

		// 定义一个接受 BindExt 的通用函数
		listUsersByAge := func(ext sqlex.BindExt, minAge int) ([]IntUser, error) {
			var users []IntUser
			err := ext.Select(&users, "SELECT * FROM int_users WHERE age > ? ORDER BY age", minAge)
			return users, err
		}

		getUserByID := func(ext sqlex.BindExt, id int) (*IntUser, error) {
			var user IntUser
			err := ext.Get(&user, "SELECT * FROM int_users WHERE id = ?", id)
			return &user, err
		}

		// 通过 DB 调用
		users, err := listUsersByAge(db, 26)
		if err != nil {
			t.Fatalf("listUsersByAge via DB failed: %v", err)
		}
		if len(users) != 2 {
			t.Errorf("expected 2 users, got %d", len(users))
		}

		user, err := getUserByID(db, 1)
		if err != nil {
			t.Fatalf("getUserByID via DB failed: %v", err)
		}
		if user.Name != "Alice" {
			t.Errorf("expected Alice, got %s", user.Name)
		}

		// 通过 Tx 调用
		tx, err := db.Beginx()
		if err != nil {
			t.Fatalf("Beginx failed: %v", err)
		}
		defer tx.Rollback()

		users, err = listUsersByAge(tx, 26)
		if err != nil {
			t.Fatalf("listUsersByAge via Tx failed: %v", err)
		}
		if len(users) != 2 {
			t.Errorf("expected 2 users, got %d", len(users))
		}

		user, err = getUserByID(tx, 2)
		if err != nil {
			t.Fatalf("getUserByID via Tx failed: %v", err)
		}
		if user.Name != "Bob" {
			t.Errorf("expected Bob, got %s", user.Name)
		}
	})
}

// 3.7 Context 版本 API
func TestIntegrationContextAPIs(t *testing.T) {
	runWithSchema(integrationSchema, t, func(db *sqlex.DB, t *testing.T, now string) {
		ctx := context.Background()
		seedIntegrationData(db, t)

		// SelectContext
		var users []IntUser
		err := db.SelectContext(ctx, &users, "SELECT * FROM int_users ORDER BY id")
		if err != nil {
			t.Fatalf("SelectContext failed: %v", err)
		}
		if len(users) != 3 {
			t.Errorf("expected 3 users, got %d", len(users))
		}

		// GetContext
		var user IntUser
		err = db.GetContext(ctx, &user, "SELECT * FROM int_users WHERE name = ?", "Bob")
		if err != nil {
			t.Fatalf("GetContext failed: %v", err)
		}
		if user.Age != 25 {
			t.Errorf("expected age 25, got %d", user.Age)
		}

		// NamedGetContext
		err = db.NamedGetContext(ctx, &user, `SELECT * FROM int_users WHERE name = :name`,
			map[string]any{"name": "Charlie"})
		if err != nil {
			t.Fatalf("NamedGetContext failed: %v", err)
		}
		if user.Age != 35 {
			t.Errorf("expected age 35, got %d", user.Age)
		}

		// NamedSelectContext
		var filtered []IntUser
		err = db.NamedSelectContext(ctx, &filtered, `SELECT * FROM int_users WHERE age >= :age ORDER BY age`,
			map[string]any{"age": 30})
		if err != nil {
			t.Fatalf("NamedSelectContext failed: %v", err)
		}
		if len(filtered) != 2 {
			t.Errorf("expected 2 users, got %d", len(filtered))
		}

		// NamedSelectContext with IN — 统一内置 IN 展开
		var selected []IntUser
		err = db.NamedSelectContext(ctx, &selected,
			`SELECT * FROM int_users WHERE name IN (:names)`,
			map[string]any{"names": []string{"Alice", "Bob"}})
		if err != nil {
			t.Fatalf("NamedSelectContext with IN failed: %v", err)
		}
		if len(selected) != 2 {
			t.Errorf("expected 2 users, got %d", len(selected))
		}
	})
}

// 3.8 事务中的 NamedGet/NamedSelect/NamedInSelect
func TestIntegrationTxNamedOperations(t *testing.T) {
	runWithSchema(integrationSchema, t, func(db *sqlex.DB, t *testing.T, now string) {
		seedIntegrationData(db, t)

		tx, err := db.Beginx()
		if err != nil {
			t.Fatalf("Beginx failed: %v", err)
		}
		defer tx.Rollback()

		// Tx.NamedGet
		var user IntUser
		err = tx.NamedGet(&user, `SELECT * FROM int_users WHERE email = :email`,
			map[string]any{"email": "alice@example.com"})
		if err != nil {
			t.Fatalf("Tx.NamedGet failed: %v", err)
		}
		if user.Name != "Alice" {
			t.Errorf("expected Alice, got %s", user.Name)
		}

		// Tx.NamedSelect
		var users []IntUser
		err = tx.NamedSelect(&users, `SELECT * FROM int_users WHERE age < :max_age ORDER BY age`,
			map[string]any{"max_age": 32})
		if err != nil {
			t.Fatalf("Tx.NamedSelect failed: %v", err)
		}
		if len(users) != 2 {
			t.Errorf("expected 2 users, got %d", len(users))
		}

		// Tx.NamedSelect with IN — 统一内置 IN 展开
		var selected []IntUser
		err = tx.NamedSelect(&selected,
			`SELECT * FROM int_users WHERE name IN (:names) ORDER BY name`,
			map[string]any{"names": []string{"Bob", "Charlie"}})
		if err != nil {
			t.Fatalf("Tx.NamedSelect with IN failed: %v", err)
		}
		if len(selected) != 2 {
			t.Errorf("expected 2 users, got %d", len(selected))
		}

		// Tx.NamedExecContext with IN —— 统一内置 IN 展开
		result, err := tx.NamedExecContext(context.Background(), `UPDATE int_users SET age = age + 1 WHERE name IN (:names)`,
			map[string]any{"names": []string{"Alice", "Bob"}})
		if err != nil {
			t.Fatalf("Tx.NamedInExec failed: %v", err)
		}
		affected, _ := result.RowsAffected()
		if affected != 2 {
			t.Errorf("expected 2 rows affected, got %d", affected)
		}
	})
}

// ========================================================
// 4. 字符串字面量中冒号不被误识别为命名参数
// ========================================================

// 4.1 命名参数查询中字符串字面量包含冒号（如 'tcp:8080'）
func TestIntegrationStringLiteralWithColon(t *testing.T) {
	var portGroupSchema = Schema{
		Create: `
CREATE TABLE port_groups (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	group_name TEXT NOT NULL,
	port TEXT NOT NULL
);
`,
		Drop: `
DROP TABLE IF EXISTS port_groups;
`,
	}

	runWithSchema(portGroupSchema, t, func(db *sqlex.DB, t *testing.T, now string) {
		// 插入种子数据
		tx := db.MustBegin()
		tx.MustExec(`INSERT INTO port_groups (group_name, port) VALUES ('web', 'tcp:8080')`)
		tx.MustExec(`INSERT INTO port_groups (group_name, port) VALUES ('web', 'tcp:443')`)
		tx.MustExec(`INSERT INTO port_groups (group_name, port) VALUES ('db', 'tcp:3306')`)
		tx.MustExec(`INSERT INTO port_groups (group_name, port) VALUES ('db', 'tcp:5432')`)
		tx.Commit()

		type PortGroup struct {
			ID        int    `db:"id"`
			GroupName string `db:"group_name"`
			Port      string `db:"port"`
		}

		// 场景一：NamedGet — 命名参数 + 字符串字面量中包含冒号
		// SQL: select * from port_groups where group_name = :group_name and port = 'tcp:8080'
		// 期望：:group_name 被识别为命名参数，'tcp:8080' 中的冒号不被误识别
		t.Run("NamedGet_StringLiteralColon", func(t *testing.T) {
			var pg PortGroup
			err := db.NamedGet(&pg,
				`SELECT * FROM port_groups WHERE group_name = :group_name AND port = 'tcp:8080'`,
				map[string]any{"group_name": "web"})
			if err != nil {
				t.Fatalf("NamedGet with string literal colon failed: %v", err)
			}
			if pg.GroupName != "web" || pg.Port != "tcp:8080" {
				t.Errorf("unexpected result: %+v", pg)
			}
		})

		// 场景二：NamedSelect — 命名参数 + 字符串字面量包含冒号
		t.Run("NamedSelect_StringLiteralColon", func(t *testing.T) {
			var results []PortGroup
			err := db.NamedSelect(&results,
				`SELECT * FROM port_groups WHERE group_name = :group_name AND port = 'tcp:8080'`,
				map[string]any{"group_name": "web"})
			if err != nil {
				t.Fatalf("NamedSelect with string literal colon failed: %v", err)
			}
			if len(results) != 1 {
				t.Fatalf("expected 1 result, got %d", len(results))
			}
			if results[0].Port != "tcp:8080" {
				t.Errorf("expected port 'tcp:8080', got '%s'", results[0].Port)
			}
		})

		// 场景三：port 也作为命名参数传入（而非硬编码字符串字面量）
		t.Run("NamedGet_BothAsParams", func(t *testing.T) {
			var pg PortGroup
			err := db.NamedGet(&pg,
				`SELECT * FROM port_groups WHERE group_name = :group_name AND port = :port`,
				map[string]any{"group_name": "db", "port": "tcp:3306"})
			if err != nil {
				t.Fatalf("NamedGet with colon in param value failed: %v", err)
			}
			if pg.GroupName != "db" || pg.Port != "tcp:3306" {
				t.Errorf("unexpected result: %+v", pg)
			}
		})

		// 场景四：位置参数 + 字符串字面量包含冒号（Get/Select 自动 Rebind）
		t.Run("Get_StringLiteralColon", func(t *testing.T) {
			var pg PortGroup
			err := db.Get(&pg,
				`SELECT * FROM port_groups WHERE group_name = ? AND port = 'tcp:8080'`, "web")
			if err != nil {
				t.Fatalf("Get with string literal colon failed: %v", err)
			}
			if pg.GroupName != "web" || pg.Port != "tcp:8080" {
				t.Errorf("unexpected result: %+v", pg)
			}
		})

		// 场景五：NamedSelect + IN + 字符串字面量包含冒号
		t.Run("NamedSelect_IN_StringLiteralColon", func(t *testing.T) {
			var results []PortGroup
			err := db.NamedSelect(&results,
				`SELECT * FROM port_groups WHERE group_name IN (:groups) AND port = 'tcp:8080'`,
				map[string]any{"groups": []string{"web", "db"}})
			if err != nil {
				t.Fatalf("NamedSelect with IN + string literal colon failed: %v", err)
			}
			if len(results) != 1 {
				t.Fatalf("expected 1 result, got %d", len(results))
			}
			if results[0].GroupName != "web" {
				t.Errorf("expected group_name 'web', got '%s'", results[0].GroupName)
			}
		})

		// 场景六：NamedExec + 字符串字面量包含冒号
		t.Run("NamedExec_StringLiteralColon", func(t *testing.T) {
			result, err := db.NamedExec(
				`UPDATE port_groups SET port = 'tcp:9090' WHERE group_name = :group_name AND port = 'tcp:8080'`,
				map[string]any{"group_name": "web"})
			if err != nil {
				t.Fatalf("NamedExec with string literal colon failed: %v", err)
			}
			rowsAff, _ := result.RowsAffected()
			if rowsAff != 1 {
				t.Errorf("expected 1 row affected, got %d", rowsAff)
			}

			// 验证更新生效
			var pg PortGroup
			err = db.NamedGet(&pg,
				`SELECT * FROM port_groups WHERE group_name = :group_name AND port = 'tcp:9090'`,
				map[string]any{"group_name": "web"})
			if err != nil {
				t.Fatalf("verify update failed: %v", err)
			}
			if pg.Port != "tcp:9090" {
				t.Errorf("expected port 'tcp:9090', got '%s'", pg.Port)
			}
		})
	})
}

// TestIntegrationConn 测试 Conn 类型的所有 Context 方法。
func TestIntegrationConn(t *testing.T) {
	runWithSchema(integrationSchema, t, func(db *sqlex.DB, t *testing.T, now string) {
		seedIntegrationData(db, t)

		ctx := context.Background()
		conn, err := db.Connx(ctx)
		if err != nil {
			t.Fatalf("Connx failed: %v", err)
		}
		defer conn.Close()

		t.Run("SelectContext", func(t *testing.T) {
			var users []IntUser
			err := conn.SelectContext(ctx, &users, "SELECT * FROM int_users ORDER BY id")
			if err != nil {
				t.Fatalf("Conn.SelectContext failed: %v", err)
			}
			if len(users) != 3 {
				t.Errorf("expected 3 users, got %d", len(users))
			}
		})

		t.Run("GetContext", func(t *testing.T) {
			var user IntUser
			err := conn.GetContext(ctx, &user, "SELECT * FROM int_users WHERE name = ?", "Alice")
			if err != nil {
				t.Fatalf("Conn.GetContext failed: %v", err)
			}
			if user.Email != "alice@example.com" {
				t.Errorf("expected alice@example.com, got %s", user.Email)
			}
		})

		t.Run("ExecContext", func(t *testing.T) {
			result, err := conn.ExecContext(ctx,
				"UPDATE int_users SET age = 99 WHERE name = ?", "Alice")
			if err != nil {
				t.Fatalf("Conn.ExecContext failed: %v", err)
			}
			rowsAff, _ := result.RowsAffected()
			if rowsAff != 1 {
				t.Errorf("expected 1 row affected, got %d", rowsAff)
			}
			// 还原
			conn.ExecContext(ctx, "UPDATE int_users SET age = 30 WHERE name = ?", "Alice")
		})

		t.Run("QueryxContext", func(t *testing.T) {
			rows, err := conn.QueryxContext(ctx, "SELECT * FROM int_users WHERE name = ?", "Bob")
			if err != nil {
				t.Fatalf("Conn.QueryxContext failed: %v", err)
			}
			defer rows.Close()

			var user IntUser
			if rows.Next() {
				err = rows.StructScan(&user)
				if err != nil {
					t.Fatalf("StructScan failed: %v", err)
				}
				if user.Name != "Bob" {
					t.Errorf("expected Bob, got %s", user.Name)
				}
			} else {
				t.Error("expected at least one row")
			}
		})

		t.Run("QueryRowxContext", func(t *testing.T) {
			row := conn.QueryRowxContext(ctx, "SELECT * FROM int_users WHERE name = ?", "Charlie")
			var user IntUser
			err := row.StructScan(&user)
			if err != nil {
				t.Fatalf("Conn.QueryRowxContext StructScan failed: %v", err)
			}
			if user.Age != 35 {
				t.Errorf("expected age 35, got %d", user.Age)
			}
		})

		t.Run("NamedGetContext", func(t *testing.T) {
			var user IntUser
			err := conn.NamedGetContext(ctx, &user,
				"SELECT * FROM int_users WHERE name = :name",
				map[string]any{"name": "Alice"})
			if err != nil {
				t.Fatalf("Conn.NamedGetContext failed: %v", err)
			}
			if user.Email != "alice@example.com" {
				t.Errorf("expected alice@example.com, got %s", user.Email)
			}
		})

		t.Run("NamedSelectContext", func(t *testing.T) {
			var users []IntUser
			err := conn.NamedSelectContext(ctx, &users,
				"SELECT * FROM int_users WHERE age >= :min_age ORDER BY age",
				map[string]any{"min_age": 25})
			if err != nil {
				t.Fatalf("Conn.NamedSelectContext failed: %v", err)
			}
			if len(users) != 3 {
				t.Errorf("expected 3 users, got %d", len(users))
			}
		})

		t.Run("NamedExecContext", func(t *testing.T) {
			result, err := conn.NamedExecContext(ctx,
				"UPDATE int_users SET age = :new_age WHERE name = :name",
				map[string]any{"name": "Bob", "new_age": 88})
			if err != nil {
				t.Fatalf("Conn.NamedExecContext failed: %v", err)
			}
			rowsAff, _ := result.RowsAffected()
			if rowsAff != 1 {
				t.Errorf("expected 1 row affected, got %d", rowsAff)
			}
			// 还原
			conn.NamedExecContext(ctx,
				"UPDATE int_users SET age = :new_age WHERE name = :name",
				map[string]any{"name": "Bob", "new_age": 25})
		})

		t.Run("NamedSelectContext_IN", func(t *testing.T) {
			var users []IntUser
			err := conn.NamedSelectContext(ctx, &users,
				"SELECT * FROM int_users WHERE name IN (:names) ORDER BY name",
				map[string]any{"names": []string{"Alice", "Charlie"}})
			if err != nil {
				t.Fatalf("Conn.NamedSelectContext with IN failed: %v", err)
			}
			if len(users) != 2 {
				t.Errorf("expected 2 users, got %d", len(users))
			}
		})
	})
}

// ========================================================
// 7. JsonValue[T] integration tests
// ========================================================

// jsonValueSchema 定义包含 JSON 列的测试表
var jsonValueSchema = Schema{
	Create: `
CREATE TABLE json_test (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	name TEXT NOT NULL,
	metadata TEXT
);
`,
	Drop: `
DROP TABLE IF EXISTS json_test;
`,
}

// JSONTestRow 用于 JsonValue 集成测试的行结构
type JSONTestRow struct {
	ID       int                    `db:"id"`
	Name     string                 `db:"name"`
	Metadata types.JsonValue[Attrs] `db:"metadata"`
}

// Attrs 是 JSON 列的内部结构
type Attrs struct {
	Role  string `json:"role"`
	Level int    `json:"level"`
}

// TestIntegrationJsonValue 验证 JsonValue[T] 与真实数据库的完整交互：
// 包括写入、读取、NULL 处理、更新等场景。
func TestIntegrationJsonValue(t *testing.T) {
	runWithSchema(jsonValueSchema, t, func(db *sqlex.DB, t *testing.T, now string) {
		// 1. 写入有效的 JSON 值
		meta := types.NewJsonValue(Attrs{Role: "admin", Level: 5})
		_, err := db.Exec("INSERT INTO json_test (name, metadata) VALUES (?, ?)",
			"alice", meta)
		if err != nil {
			t.Fatalf("insert with JsonValue failed: %v", err)
		}

		// 2. 写入 NULL JSON 值
		var nullMeta types.JsonValue[Attrs]
		_, err = db.Exec("INSERT INTO json_test (name, metadata) VALUES (?, ?)",
			"bob", nullMeta)
		if err != nil {
			t.Fatalf("insert with null JsonValue failed: %v", err)
		}

		// 3. 读取有效的 JSON 值
		var row JSONTestRow
		err = db.Get(&row, "SELECT * FROM json_test WHERE name = ?", "alice")
		if err != nil {
			t.Fatalf("Get JsonValue row failed: %v", err)
		}
		if !row.Metadata.Valid {
			t.Error("expected Metadata.Valid=true for alice")
		}
		if row.Metadata.Val.Role != "admin" {
			t.Errorf("expected role=admin, got %s", row.Metadata.Val.Role)
		}
		if row.Metadata.Val.Level != 5 {
			t.Errorf("expected level=5, got %d", row.Metadata.Val.Level)
		}

		// 4. 读取 NULL JSON 值
		var nullRow JSONTestRow
		err = db.Get(&nullRow, "SELECT * FROM json_test WHERE name = ?", "bob")
		if err != nil {
			t.Fatalf("Get null JsonValue row failed: %v", err)
		}
		if nullRow.Metadata.Valid {
			t.Error("expected Metadata.Valid=false for bob (NULL)")
		}

		// 5. 使用 NamedExec 更新 JSON 值
		updated := types.NewJsonValue(Attrs{Role: "user", Level: 2})
		_, err = db.NamedExec("UPDATE json_test SET metadata = :metadata WHERE name = :name",
			map[string]any{"name": "bob", "metadata": updated})
		if err != nil {
			t.Fatalf("NamedExec update JsonValue failed: %v", err)
		}

		// 6. 验证更新结果
		var updatedRow JSONTestRow
		err = db.Get(&updatedRow, "SELECT * FROM json_test WHERE name = ?", "bob")
		if err != nil {
			t.Fatalf("Get updated JsonValue row failed: %v", err)
		}
		if !updatedRow.Metadata.Valid {
			t.Error("expected Metadata.Valid=true after update")
		}
		if updatedRow.Metadata.Val.Role != "user" || updatedRow.Metadata.Val.Level != 2 {
			t.Errorf("expected {user, 2}, got {%s, %d}",
				updatedRow.Metadata.Val.Role, updatedRow.Metadata.Val.Level)
		}

		// 7. Select 多行 — 混合 NULL 和非 NULL
		var rows []JSONTestRow
		err = db.Select(&rows, "SELECT * FROM json_test ORDER BY id")
		if err != nil {
			t.Fatalf("Select JsonValue rows failed: %v", err)
		}
		if len(rows) != 2 {
			t.Fatalf("expected 2 rows, got %d", len(rows))
		}
		// alice 有值，bob 也已更新为有值
		for _, r := range rows {
			if !r.Metadata.Valid {
				t.Errorf("expected all metadata valid after update, row %s is invalid", r.Name)
			}
		}

	})
}

// ========================================================
// 9. MapperFunc 修改后的行为测试
// ========================================================

// TestIntegrationMapperFunc 验证 DB.MapperFunc 修改后，
// Select/Get 能按照新的映射规则正确扫描字段。
func TestIntegrationMapperFunc(t *testing.T) {
	runWithSchema(integrationSchema, t, func(db *sqlex.DB, t *testing.T, now string) {
		seedIntegrationData(db, t)

		// 测试默认 Mapper（小写映射）正常工作
		t.Run("DefaultMapper", func(t *testing.T) {
			var user IntUser
			err := db.Get(&user, "SELECT * FROM int_users WHERE name = ?", "Alice")
			if err != nil {
				t.Fatalf("Get with default mapper failed: %v", err)
			}
			if user.Name != "Alice" {
				t.Errorf("expected Alice, got %s", user.Name)
			}
		})

		// 测试 MapperFunc 切换映射策略
		t.Run("CustomMapper", func(t *testing.T) {
			// 使用 json tag 作为映射
			db.MapperFunc(strings.ToLower)
			var user IntUser
			err := db.Get(&user, "SELECT * FROM int_users WHERE name = ?", "Bob")
			if err != nil {
				t.Fatalf("Get with custom mapper failed: %v", err)
			}
			if user.Name != "Bob" {
				t.Errorf("expected Bob, got %s", user.Name)
			}
		})

		// 测试独立 DB 副本的 MapperFunc 不影响原 DB
		t.Run("IndependentMapper", func(t *testing.T) {
			// 创建一个新的 DB 连接，使用不同的 Mapper
			dbCopy := sqlex.NewDb(db.DB, db.DriverName())
			dbCopy.MapperFunc(strings.ToUpper)

		// With an uppercase mapper, struct fields need uppercase db tags to match.
		// This test verifies: original db is unaffected by dbCopy's mapper change.
		var user IntUser
			err := db.Get(&user, "SELECT * FROM int_users WHERE name = ?", "Charlie")
			if err != nil {
				t.Fatalf("Original db should not be affected by dbCopy: %v", err)
			}
			if user.Name != "Charlie" {
				t.Errorf("expected Charlie, got %s", user.Name)
			}

			// dbCopy 正常功能验证（使用 Exec 不依赖 Mapper）
			_, err = dbCopy.Exec("UPDATE int_users SET age = 99 WHERE name = ?", "Charlie")
			if err != nil {
				t.Fatalf("dbCopy Exec should work: %v", err)
			}
			// 还原
			dbCopy.Exec("UPDATE int_users SET age = 35 WHERE name = ?", "Charlie")
		})
	})
}
