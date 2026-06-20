// concurrent_test.go — concurrency stress tests
//
// Verifies connection pool stability and data correctness under high concurrency.
package cross_db_test

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"

	sqlex "github.com/go-sqlex/sqlex"
)

// TestConcurrentSelect — 100 goroutine 并发 Select
func TestConcurrentSelect(t *testing.T) {
	runWithSchema(crossSchema, t, func(db *sqlex.DB, t *testing.T, now string) {
		crossDBOnly(t)
		seedCrossData(db, t)

		const goroutines = 100
		var wg sync.WaitGroup
		var errCount atomic.Int64

		wg.Add(goroutines)
		for i := 0; i < goroutines; i++ {
			go func() {
				defer wg.Done()
				var users []CrossUser
				err := db.SelectContext(context.Background(), &users, "SELECT * FROM cross_users ORDER BY id")
				if err != nil {
					errCount.Add(1)
					return
				}
				if len(users) != 3 {
					errCount.Add(1)
				}
			}()
		}
		wg.Wait()

		if errCount.Load() > 0 {
			t.Errorf("[%s] %d/%d concurrent Select failed", dbLabel(db), errCount.Load(), goroutines)
		}
	})
}

// TestConcurrentExec — 50 goroutine 并发 Exec
func TestConcurrentExec(t *testing.T) {
	runWithSchema(crossSchema, t, func(db *sqlex.DB, t *testing.T, now string) {
		crossDBOnly(t)
		seedCrossData(db, t)

		const goroutines = 50
		var wg sync.WaitGroup
		var errCount atomic.Int64

		wg.Add(goroutines)
		for i := 0; i < goroutines; i++ {
			go func(idx int) {
				defer wg.Done()
				_, err := db.ExecContext(context.Background(),
					"INSERT INTO cross_orders (user_id, amount, status) VALUES (?, ?, ?)",
					1, float64(idx), "test")
				if err != nil {
					errCount.Add(1)
				}
			}(i)
		}
		wg.Wait()

		if errCount.Load() > 0 {
			t.Errorf("[%s] %d/%d concurrent Exec failed", dbLabel(db), errCount.Load(), goroutines)
		}

		// 验证数据完整性
		var count int
		err := db.GetContext(context.Background(), &count, "SELECT COUNT(*) FROM cross_orders WHERE status = ?", "test")
		if err != nil {
			t.Fatalf("[%s] count query failed: %v", dbLabel(db), err)
		}
		if count != goroutines {
			t.Errorf("[%s] expected %d rows, got %d", dbLabel(db), goroutines, count)
		}
	})
}

// TestConcurrentNamedQuery — 50 goroutine 并发 NamedSelect
func TestConcurrentNamedQuery(t *testing.T) {
	runWithSchema(crossSchema, t, func(db *sqlex.DB, t *testing.T, now string) {
		crossDBOnly(t)
		seedCrossData(db, t)

		const goroutines = 50
		var wg sync.WaitGroup
		var errCount atomic.Int64

		wg.Add(goroutines)
		for i := 0; i < goroutines; i++ {
			go func() {
				defer wg.Done()
				var users []CrossUser
				err := db.NamedSelectContext(context.Background(), &users,
					"SELECT * FROM cross_users WHERE age > :min_age",
					map[string]any{"min_age": 20})
				if err != nil {
					errCount.Add(1)
					return
				}
				if len(users) != 3 {
					errCount.Add(1)
				}
			}()
		}
		wg.Wait()

		if errCount.Load() > 0 {
			t.Errorf("[%s] %d/%d concurrent NamedSelect failed", dbLabel(db), errCount.Load(), goroutines)
		}
	})
}

// TestConcurrentHook — 并发查询时 Hook 正确触发（线程安全）
func TestConcurrentHook(t *testing.T) {
	runWithSchema(crossSchema, t, func(db *sqlex.DB, t *testing.T, now string) {
		crossDBOnly(t)
		seedCrossData(db, t)

		dbCopy := sqlex.NewDb(db.DB, db.DriverName())
		hook := &crossTestHook{}
		dbCopy.AddHook(hook)

		const goroutines = 50
		var wg sync.WaitGroup

		wg.Add(goroutines)
		for i := 0; i < goroutines; i++ {
			go func() {
				defer wg.Done()
				var user CrossUser
				dbCopy.GetContext(context.Background(), &user, "SELECT * FROM cross_users WHERE name = ?", "Alice")
			}()
		}
		wg.Wait()

		hook.mu.Lock()
		defer hook.mu.Unlock()

		if hook.beforeCount != goroutines {
			t.Errorf("[%s] expected %d BeforeQuery calls, got %d", dbLabel(db), goroutines, hook.beforeCount)
		}
		if hook.afterCount != goroutines {
			t.Errorf("[%s] expected %d AfterQuery calls, got %d", dbLabel(db), goroutines, hook.afterCount)
		}
	})
}

// 抑制 unused import
var _ = fmt.Sprintf
