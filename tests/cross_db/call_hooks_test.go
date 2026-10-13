// call_hooks_test.go — WithCallHooks（方法级/调用级 Hook）跨数据库集成测试
//
// 覆盖点：
//   - 调用级 Hook 只在 *Context 方法上生效，非 Context 方法不受影响
//   - 与全局 Hook 共存时的洋葱顺序（全局在外，调用级在内）
//   - 调用间隔离：不污染 DB 实例的全局 Hook，不泄漏到其他 ctx
//   - 并发场景下各 goroutine 携带不同 Hook 互不串扰
//   - Tx 不捕获调用级 Hook（需显式传 ctx），Stmt 在执行期合并
//   - 端到端动态路由：按调用改写目标表 / schema —— 即"动态指定 schema"的落地形态
package cross_db_test

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"

	sqlex "github.com/go-sqlex/sqlex"
)

// ---------- 测试用 Schema：两张结构相同的"租户"表 ----------

var callHookSchema = Schema{
	Create: `
CREATE TABLE ch_tenant_a (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	name TEXT NOT NULL
);

CREATE TABLE ch_tenant_b (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	name TEXT NOT NULL
);
`,
	Drop: `
DROP TABLE IF EXISTS ch_tenant_a;
DROP TABLE IF EXISTS ch_tenant_b;
`,
}

// logicalTable 是业务侧书写的逻辑表名，由调用级 Hook 在运行期解析为物理表
const logicalTable = "ch_tenant"

// seedCallHookData 为两张租户表写入可区分的数据
func seedCallHookData(db *sqlex.DB, t *testing.T) {
	t.Helper()
	tx := db.MustBegin()
	tx.MustExec(`INSERT INTO ch_tenant_a (name) VALUES ('from-a')`)
	tx.MustExec(`INSERT INTO ch_tenant_b (name) VALUES ('from-b')`)
	if err := tx.Commit(); err != nil {
		t.Fatalf("[%s] seed commit failed: %v", dbLabel(db), err)
	}
}

// ---------- 测试用 Hook 实现 ----------

// countingHook 记录调用次数与观察到的 SQL，可并发使用
type countingHook struct {
	name   string
	mu     sync.Mutex
	before int
	after  int
	seen   []string
	order  *[]string // 可选：与其他 Hook 共享，用于断言洋葱顺序
	orderM *sync.Mutex
}

func (h *countingHook) BeforeQuery(ctx context.Context, event *sqlex.QueryEvent) context.Context {
	h.mu.Lock()
	h.before++
	h.seen = append(h.seen, event.Query)
	h.mu.Unlock()
	h.record("before:" + h.name)
	return ctx
}

func (h *countingHook) AfterQuery(ctx context.Context, event *sqlex.QueryEvent) {
	h.mu.Lock()
	h.after++
	h.mu.Unlock()
	h.record("after:" + h.name)
}

func (h *countingHook) record(entry string) {
	if h.order == nil {
		return
	}
	h.orderM.Lock()
	defer h.orderM.Unlock()
	*h.order = append(*h.order, entry)
}

func (h *countingHook) counts() (int, int) {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.before, h.after
}

func (h *countingHook) queries() []string {
	h.mu.Lock()
	defer h.mu.Unlock()
	return append([]string(nil), h.seen...)
}

// tableRouteHook 把逻辑表名改写为指定物理表，模拟按调用动态指定 schema / 分表。
// 它是无状态的，因此可安全地被多个 goroutine 共享。
type tableRouteHook struct {
	physical string
}

func (h *tableRouteHook) BeforeQuery(ctx context.Context, event *sqlex.QueryEvent) context.Context {
	event.Query = strings.ReplaceAll(event.Query, logicalTable, h.physical)
	return ctx
}

func (h *tableRouteHook) AfterQuery(context.Context, *sqlex.QueryEvent) {}

// ========================================================
// TestCrossDBCallHooksContextMethodsOnly
// 调用级 Hook 在各 *Context 方法上均触发；非 Context 方法不触发
// ========================================================
func TestCrossDBCallHooksContextMethodsOnly(t *testing.T) {
	runWithSchema(callHookSchema, t, func(db *sqlex.DB, t *testing.T, now string) {
		crossDBOnly(t)
		seedCallHookData(db, t)

		hook := &countingHook{name: "call"}
		ctx := sqlex.WithCallHooks(context.Background(), hook)

		var name string
		if err := db.GetContext(ctx, &name, "SELECT name FROM ch_tenant_a WHERE id = ?", 1); err != nil {
			t.Fatalf("[%s] GetContext failed: %v", dbLabel(db), err)
		}

		var names []string
		if err := db.SelectContext(ctx, &names, "SELECT name FROM ch_tenant_a ORDER BY id"); err != nil {
			t.Fatalf("[%s] SelectContext failed: %v", dbLabel(db), err)
		}

		if _, err := db.ExecContext(ctx, "UPDATE ch_tenant_a SET name = ? WHERE id = ?", "from-a", 1); err != nil {
			t.Fatalf("[%s] ExecContext failed: %v", dbLabel(db), err)
		}

		if _, err := db.NamedExecContext(ctx,
			"UPDATE ch_tenant_a SET name = :name WHERE id = :id",
			map[string]any{"name": "from-a", "id": 1}); err != nil {
			t.Fatalf("[%s] NamedExecContext failed: %v", dbLabel(db), err)
		}

		before, after := hook.counts()
		const wantCalls = 4
		if before != wantCalls || after != wantCalls {
			t.Errorf("[%s] expected %d before/after calls, got %d/%d",
				dbLabel(db), wantCalls, before, after)
		}

		// 非 Context 方法走 context.Background()，不应看到调用级 Hook
		var ignored []string
		if err := db.Select(&ignored, "SELECT name FROM ch_tenant_a ORDER BY id"); err != nil {
			t.Fatalf("[%s] Select failed: %v", dbLabel(db), err)
		}
		if before, after = hook.counts(); before != wantCalls || after != wantCalls {
			t.Errorf("[%s] non-context method must not trigger call-scoped hooks, got %d/%d",
				dbLabel(db), before, after)
		}
	})
}

// ========================================================
// TestCrossDBCallHooksOnionOrder
// 全局 Hook 包裹调用级 Hook：Before 全局先，After 全局后
// ========================================================
func TestCrossDBCallHooksOnionOrder(t *testing.T) {
	runWithSchema(callHookSchema, t, func(db *sqlex.DB, t *testing.T, now string) {
		crossDBOnly(t)
		seedCallHookData(db, t)

		// 独立包装，避免污染共享 DB 的全局 Hook
		dbCopy := sqlex.NewDB(db.DB, db.DriverName())
		var order []string
		var mu sync.Mutex

		globalHook := &countingHook{name: "global", order: &order, orderM: &mu}
		callHook := &countingHook{name: "call", order: &order, orderM: &mu}
		dbCopy.AddHook(globalHook)

		ctx := sqlex.WithCallHooks(context.Background(), callHook)
		var name string
		if err := dbCopy.GetContext(ctx, &name, "SELECT name FROM ch_tenant_a WHERE id = ?", 1); err != nil {
			t.Fatalf("[%s] GetContext failed: %v", dbLabel(db), err)
		}

		mu.Lock()
		defer mu.Unlock()
		want := []string{"before:global", "before:call", "after:call", "after:global"}
		if len(order) != len(want) {
			t.Fatalf("[%s] expected %d hook events, got %d: %v", dbLabel(db), len(want), len(order), order)
		}
		for i, exp := range want {
			if order[i] != exp {
				t.Errorf("[%s] hook event %d: got %q, want %q", dbLabel(db), i, order[i], exp)
			}
		}
	})
}

// ========================================================
// TestCrossDBCallHooksIsolation
// 调用级 Hook 不污染 DB 全局 Hook，也不泄漏到未携带它的 ctx
// ========================================================
func TestCrossDBCallHooksIsolation(t *testing.T) {
	runWithSchema(callHookSchema, t, func(db *sqlex.DB, t *testing.T, now string) {
		crossDBOnly(t)
		seedCallHookData(db, t)

		dbCopy := sqlex.NewDB(db.DB, db.DriverName())
		globalHook := &countingHook{name: "global"}
		callHook := &countingHook{name: "call"}
		dbCopy.AddHook(globalHook)

		query := "SELECT name FROM ch_tenant_a WHERE id = ?"
		var name string

		// 第 1 次：携带调用级 Hook
		ctxWithHook := sqlex.WithCallHooks(context.Background(), callHook)
		if err := dbCopy.GetContext(ctxWithHook, &name, query, 1); err != nil {
			t.Fatalf("[%s] GetContext with call hook failed: %v", dbLabel(db), err)
		}

		// 第 2 次：干净 ctx
		if err := dbCopy.GetContext(context.Background(), &name, query, 1); err != nil {
			t.Fatalf("[%s] GetContext without call hook failed: %v", dbLabel(db), err)
		}

		if before, _ := globalHook.counts(); before != 2 {
			t.Errorf("[%s] global hook should fire on both queries, got %d", dbLabel(db), before)
		}
		if before, _ := callHook.counts(); before != 1 {
			t.Errorf("[%s] call-scoped hook should fire once, got %d", dbLabel(db), before)
		}
	})
}

// ========================================================
// TestCrossDBCallHooksDynamicRouting
// 端到端：同一条逻辑 SQL 按调用级 Hook 路由到不同物理表
// 这是"动态指定 schema"的落地形态
// ========================================================
func TestCrossDBCallHooksDynamicRouting(t *testing.T) {
	runWithSchema(callHookSchema, t, func(db *sqlex.DB, t *testing.T, now string) {
		crossDBOnly(t)
		seedCallHookData(db, t)

		// 业务侧只书写逻辑表名，物理表由调用级 Hook 决定
		query := fmt.Sprintf("SELECT name FROM %s ORDER BY id", logicalTable)

		cases := []struct {
			physical string
			want     string
		}{
			{physical: "ch_tenant_a", want: "from-a"},
			{physical: "ch_tenant_b", want: "from-b"},
		}

		for _, c := range cases {
			ctx := sqlex.WithCallHooks(context.Background(), &tableRouteHook{physical: c.physical})
			var got string
			if err := db.GetContext(ctx, &got, query); err != nil {
				t.Fatalf("[%s] routing to %s failed: %v", dbLabel(db), c.physical, err)
			}
			if got != c.want {
				t.Errorf("[%s] routing to %s: got %q, want %q", dbLabel(db), c.physical, got, c.want)
			}
		}

		// 不携带路由 Hook 时，逻辑表名不存在，应当报错而不是静默命中某张表
		var got string
		if err := db.GetContext(context.Background(), &got, query); err == nil {
			t.Errorf("[%s] query against unresolved logical table should fail", dbLabel(db))
		}
	})
}

// ========================================================
// TestCrossDBCallHooksRoutingWithArgs
// 改写 SQL 与参数绑定共存：占位符数量不变，args 正常生效
// ========================================================
func TestCrossDBCallHooksRoutingWithArgs(t *testing.T) {
	runWithSchema(callHookSchema, t, func(db *sqlex.DB, t *testing.T, now string) {
		crossDBOnly(t)
		seedCallHookData(db, t)

		query := fmt.Sprintf("SELECT name FROM %s WHERE id = ? AND name <> ?", logicalTable)
		ctx := sqlex.WithCallHooks(context.Background(), &tableRouteHook{physical: "ch_tenant_b"})

		var got string
		if err := db.GetContext(ctx, &got, query, 1, "unused"); err != nil {
			t.Fatalf("[%s] GetContext with args failed: %v", dbLabel(db), err)
		}
		if got != "from-b" {
			t.Errorf("[%s] got %q, want %q", dbLabel(db), got, "from-b")
		}
	})
}

// ========================================================
// TestCrossDBCallHooksConcurrentRouting
// 并发：各 goroutine 携带各自的路由 Hook，互不串扰
// ========================================================
func TestCrossDBCallHooksConcurrentRouting(t *testing.T) {
	runWithSchema(callHookSchema, t, func(db *sqlex.DB, t *testing.T, now string) {
		crossDBOnly(t)
		seedCallHookData(db, t)

		// SQLite 共享内存库并发能力有限，跳过以避免 database is locked 干扰
		if isSQLite(db) {
			return
		}

		query := fmt.Sprintf("SELECT name FROM %s ORDER BY id", logicalTable)
		const iterations = 20

		type outcome struct {
			physical string
			got      string
			err      error
		}
		results := make([]outcome, iterations*2)

		var wg sync.WaitGroup
		for i := 0; i < iterations; i++ {
			for j, physical := range []string{"ch_tenant_a", "ch_tenant_b"} {
				wg.Add(1)
				idx := i*2 + j
				go func(idx int, physical string) {
					defer wg.Done()
					ctx := sqlex.WithCallHooks(context.Background(), &tableRouteHook{physical: physical})
					var got string
					err := db.GetContext(ctx, &got, query)
					results[idx] = outcome{physical: physical, got: got, err: err}
				}(idx, physical)
			}
		}
		wg.Wait()

		wantFor := map[string]string{"ch_tenant_a": "from-a", "ch_tenant_b": "from-b"}
		for _, r := range results {
			if r.err != nil {
				t.Fatalf("[%s] concurrent query on %s failed: %v", dbLabel(db), r.physical, r.err)
			}
			if want := wantFor[r.physical]; r.got != want {
				t.Errorf("[%s] concurrent routing to %s: got %q, want %q",
					dbLabel(db), r.physical, r.got, want)
			}
		}
	})
}

// ========================================================
// TestCrossDBCallHooksTxNotCaptured
// Tx 不捕获 BeginTxx 时 ctx 中的调用级 Hook；需显式传 ctx 才生效（不重复执行）
// ========================================================
func TestCrossDBCallHooksTxNotCaptured(t *testing.T) {
	runWithSchema(callHookSchema, t, func(db *sqlex.DB, t *testing.T, now string) {
		crossDBOnly(t)
		seedCallHookData(db, t)

		hook := &countingHook{name: "call"}
		ctx := sqlex.WithCallHooks(context.Background(), hook)

		tx, err := db.BeginTxx(ctx, nil)
		if err != nil {
			t.Fatalf("[%s] BeginTxx failed: %v", dbLabel(db), err)
		}
		defer tx.Rollback()

		// BeginTxx 自身触发一次 OpBegin
		if before, _ := hook.counts(); before != 1 {
			t.Fatalf("[%s] BeginTxx should trigger the call-scoped hook once, got %d", dbLabel(db), before)
		}

		// 未传 ctx：Tx 不应继承调用级 Hook
		var name string
		if err = tx.GetContext(context.Background(), &name, "SELECT name FROM ch_tenant_a WHERE id = ?", 1); err != nil {
			t.Fatalf("[%s] Tx.GetContext with clean ctx failed: %v", dbLabel(db), err)
		}
		if before, _ := hook.counts(); before != 1 {
			t.Errorf("[%s] Tx must not capture call-scoped hooks, got %d", dbLabel(db), before)
		}

		// 传同一个 ctx：生效且只执行一次
		if err = tx.GetContext(ctx, &name, "SELECT name FROM ch_tenant_a WHERE id = ?", 1); err != nil {
			t.Fatalf("[%s] Tx.GetContext with call ctx failed: %v", dbLabel(db), err)
		}
		before, after := hook.counts()
		if before != 2 || after != 2 {
			t.Errorf("[%s] expected exactly 2 before/after calls (no double firing), got %d/%d",
				dbLabel(db), before, after)
		}
	})
}

// ========================================================
// TestCrossDBCallHooksTxRouting
// 事务内按 ctx 动态路由，验证调用级 Hook 在 Tx 上的实际可用性
// ========================================================
func TestCrossDBCallHooksTxRouting(t *testing.T) {
	runWithSchema(callHookSchema, t, func(db *sqlex.DB, t *testing.T, now string) {
		crossDBOnly(t)
		seedCallHookData(db, t)

		ctx := sqlex.WithCallHooks(context.Background(), &tableRouteHook{physical: "ch_tenant_b"})

		tx, err := db.BeginTxx(ctx, nil)
		if err != nil {
			t.Fatalf("[%s] BeginTxx failed: %v", dbLabel(db), err)
		}
		defer tx.Rollback()

		var got string
		query := fmt.Sprintf("SELECT name FROM %s ORDER BY id", logicalTable)
		if err = tx.GetContext(ctx, &got, query); err != nil {
			t.Fatalf("[%s] Tx routing failed: %v", dbLabel(db), err)
		}
		if got != "from-b" {
			t.Errorf("[%s] Tx routing: got %q, want %q", dbLabel(db), got, "from-b")
		}
	})
}

// ========================================================
// TestCrossDBCallHooksTxCloseWithErrContext
// CloseWithErrContext 将 ctx 透传给 commit/rollback Hook，Tx 无全局 Hook 时调用级 Hook 也生效（#7 F-003）
// ========================================================
func TestCrossDBCallHooksTxCloseWithErrContext(t *testing.T) {
	runWithSchema(callHookSchema, t, func(db *sqlex.DB, t *testing.T, now string) {
		crossDBOnly(t)

		for wantOp, bizErr := range map[sqlex.OpType]error{
			sqlex.OpCommit:   nil,
			sqlex.OpRollback: errors.New("biz failed"),
		} {
			hook := &countingHook{name: "call"}
			if tx, err := db.Beginx(); err != nil { // db has no global Hooks
				t.Fatalf("[%s] Beginx failed: %v", dbLabel(db), err)
			} else {
				tx.CloseWithErrContext(sqlex.WithCallHooks(context.Background(), hook), bizErr)
			}

			if q := hook.queries(); len(q) != 1 || q[0] != "TX "+string(wantOp) {
				t.Errorf("[%s] call-scoped hook should see [TX %s] once, got %v", dbLabel(db), wantOp, q)
			}
		}
	})
}

// ========================================================
// TestCrossDBCallHooksStmt
// Stmt 在执行期合并调用级 Hook（预编译期不固化）
// ========================================================
func TestCrossDBCallHooksStmt(t *testing.T) {
	runWithSchema(callHookSchema, t, func(db *sqlex.DB, t *testing.T, now string) {
		crossDBOnly(t)
		seedCallHookData(db, t)

		dbCopy := sqlex.NewDB(db.DB, db.DriverName())
		globalHook := &countingHook{name: "global"}
		dbCopy.AddHook(globalHook)

		callHook := &countingHook{name: "call"}
		// 预编译时携带调用级 Hook，验证它不会被固化进 Stmt
		prepareCtx := sqlex.WithCallHooks(context.Background(), callHook)
		stmt, err := dbCopy.PreparexContext(prepareCtx, "SELECT name FROM ch_tenant_a WHERE id = ?")
		if err != nil {
			t.Fatalf("[%s] PreparexContext failed: %v", dbLabel(db), err)
		}
		defer stmt.Close()

		callBeforePrepare, _ := callHook.counts()

		// 干净 ctx 执行：只触发继承来的全局 Hook
		var name string
		if err = stmt.GetContext(context.Background(), &name, 1); err != nil {
			t.Fatalf("[%s] Stmt.GetContext failed: %v", dbLabel(db), err)
		}
		if before, _ := callHook.counts(); before != callBeforePrepare {
			t.Errorf("[%s] call-scoped hook must not be baked into Stmt, got %d (was %d)",
				dbLabel(db), before, callBeforePrepare)
		}
		if before, _ := globalHook.counts(); before == 0 {
			t.Errorf("[%s] Stmt should inherit global hooks", dbLabel(db))
		}

		// 携带调用级 Hook 执行：本次生效
		if err = stmt.GetContext(prepareCtx, &name, 1); err != nil {
			t.Fatalf("[%s] Stmt.GetContext with call ctx failed: %v", dbLabel(db), err)
		}
		if before, _ := callHook.counts(); before != callBeforePrepare+1 {
			t.Errorf("[%s] call-scoped hook should fire on Stmt exec, got %d (was %d)",
				dbLabel(db), before, callBeforePrepare)
		}
	})
}

// ========================================================
// TestCrossDBCallHooksInnermostWins
// 嵌套 WithCallHooks 采用覆盖语义：最内层（最具体）的设置生效
// ========================================================
func TestCrossDBCallHooksInnermostWins(t *testing.T) {
	runWithSchema(callHookSchema, t, func(db *sqlex.DB, t *testing.T, now string) {
		crossDBOnly(t)
		seedCallHookData(db, t)

		query := fmt.Sprintf("SELECT name FROM %s ORDER BY id", logicalTable)

		outerCtx := sqlex.WithCallHooks(context.Background(), &tableRouteHook{physical: "ch_tenant_a"})
		innerCtx := sqlex.WithCallHooks(outerCtx, &tableRouteHook{physical: "ch_tenant_b"})

		var got string
		if err := db.GetContext(innerCtx, &got, query); err != nil {
			t.Fatalf("[%s] inner ctx query failed: %v", dbLabel(db), err)
		}
		if got != "from-b" {
			t.Errorf("[%s] innermost hook should win: got %q, want %q", dbLabel(db), got, "from-b")
		}

		// 外层 ctx 不受内层影响，仍路由到 a
		if err := db.GetContext(outerCtx, &got, query); err != nil {
			t.Fatalf("[%s] outer ctx query failed: %v", dbLabel(db), err)
		}
		if got != "from-a" {
			t.Errorf("[%s] outer ctx must stay intact: got %q, want %q", dbLabel(db), got, "from-a")
		}
	})
}

// ========================================================
// TestCrossDBCallHooksExplicitComposition
// 覆盖语义下由调用方一处组合多个 Hook，链上全部生效
// ========================================================
func TestCrossDBCallHooksExplicitComposition(t *testing.T) {
	runWithSchema(callHookSchema, t, func(db *sqlex.DB, t *testing.T, now string) {
		crossDBOnly(t)
		seedCallHookData(db, t)

		observer := &countingHook{name: "observer"}
		router := &tableRouteHook{physical: "ch_tenant_b"}

		// 调用方显式组合：观测 Hook + 路由 Hook
		ctx := sqlex.WithCallHooks(context.Background(), observer, router)

		var got string
		query := fmt.Sprintf("SELECT name FROM %s ORDER BY id", logicalTable)
		if err := db.GetContext(ctx, &got, query); err != nil {
			t.Fatalf("[%s] composed hooks query failed: %v", dbLabel(db), err)
		}
		if got != "from-b" {
			t.Errorf("[%s] router hook should apply: got %q, want %q", dbLabel(db), got, "from-b")
		}

		before, after := observer.counts()
		if before != 1 || after != 1 {
			t.Errorf("[%s] observer hook should fire once, got %d/%d", dbLabel(db), before, after)
		}
		// observer 排在 router 之前，观察到的是改写前的逻辑表名
		if seen := observer.queries(); len(seen) != 1 || !strings.Contains(seen[0], logicalTable) {
			t.Errorf("[%s] observer should see the pre-rewrite SQL, got %v", dbLabel(db), seen)
		}
	})
}

// ========================================================
// TestCrossDBCallHooksObserveFinalSQL
// 全局 Hook 的 BeforeQuery 早于调用级改写，观察到改写前的 SQL
// ========================================================
func TestCrossDBCallHooksObserveFinalSQL(t *testing.T) {
	runWithSchema(callHookSchema, t, func(db *sqlex.DB, t *testing.T, now string) {
		crossDBOnly(t)
		seedCallHookData(db, t)

		dbCopy := sqlex.NewDB(db.DB, db.DriverName())
		globalHook := &countingHook{name: "global"}
		dbCopy.AddHook(globalHook)

		query := fmt.Sprintf("SELECT name FROM %s ORDER BY id", logicalTable)
		ctx := sqlex.WithCallHooks(context.Background(), &tableRouteHook{physical: "ch_tenant_b"})

		var got string
		if err := dbCopy.GetContext(ctx, &got, query); err != nil {
			t.Fatalf("[%s] GetContext failed: %v", dbLabel(db), err)
		}

		// 全局 Hook 的 BeforeQuery 早于调用级改写，看到的是逻辑表名
		seen := globalHook.queries()
		if len(seen) == 0 {
			t.Fatalf("[%s] global hook recorded no query", dbLabel(db))
		}
		if !strings.Contains(seen[0], logicalTable) {
			t.Errorf("[%s] global BeforeQuery should observe the pre-rewrite SQL, got %q",
				dbLabel(db), seen[0])
		}
	})
}
