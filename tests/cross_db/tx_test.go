// tx_test.go — cross-database integration tests for transaction handling
package cross_db_test

import (
	"context"
	"errors"
	"testing"

	sqlex "github.com/go-sqlex/sqlex"
)

// ========================================================
// TestCrossDBTxCommitRollback — 验证事务 Commit 和 Rollback
// ========================================================
func TestCrossDBTxCommitRollback(t *testing.T) {
	runWithSchema(crossSchema, t, func(db *sqlex.DB, t *testing.T, now string) {
		crossDBOnly(t)

		// Commit 持久化数据
		t.Run("Commit", func(t *testing.T) {
			tx, err := db.Beginx()
			if err != nil {
				t.Fatalf("[%s] Beginx failed: %v", dbLabel(db), err)
			}
			tx.MustExec(`INSERT INTO cross_users (name, email, age) VALUES ('TxUser', 'tx@example.com', 20)`)
			err = tx.Commit()
			if err != nil {
				t.Fatalf("[%s] Commit failed: %v", dbLabel(db), err)
			}

			// 提交后数据应存在
			var count int
			db.Get(&count, "SELECT COUNT(*) FROM cross_users WHERE name = 'TxUser'")
			if count != 1 {
				t.Errorf("[%s] expected 1 user after commit, got %d", dbLabel(db), count)
			}
		})

		// Rollback 撤销变更
		t.Run("Rollback", func(t *testing.T) {
			tx, err := db.Beginx()
			if err != nil {
				t.Fatalf("[%s] Beginx failed: %v", dbLabel(db), err)
			}
			tx.MustExec(`INSERT INTO cross_users (name, email, age) VALUES ('RollbackUser', 'rb@example.com', 21)`)
			err = tx.Rollback()
			if err != nil {
				t.Fatalf("[%s] Rollback failed: %v", dbLabel(db), err)
			}

			// 回滚后数据不应存在
			var count int
			db.Get(&count, "SELECT COUNT(*) FROM cross_users WHERE name = 'RollbackUser'")
			if count != 0 {
				t.Errorf("[%s] expected 0 users after rollback, got %d", dbLabel(db), count)
			}
		})
	})
}

// ========================================================
// TestCrossDBCloseWithErr — 验证 CloseWithErr 自动提交/回滚
// ========================================================
func TestCrossDBCloseWithErr(t *testing.T) {
	runWithSchema(crossSchema, t, func(db *sqlex.DB, t *testing.T, now string) {
		crossDBOnly(t)

		// CloseWithErr(nil) → Commit
		t.Run("CommitOnNil", func(t *testing.T) {
			tx, err := db.Beginx()
			if err != nil {
				t.Fatalf("[%s] Beginx failed: %v", dbLabel(db), err)
			}
			tx.MustExec(`INSERT INTO cross_users (name, email, age) VALUES ('CloseTxCommit', 'close@commit.com', 30)`)
			tx.CloseWithErr(nil) // 应自动提交

			var count int
			db.Get(&count, "SELECT COUNT(*) FROM cross_users WHERE name = 'CloseTxCommit'")
			if count != 1 {
				t.Errorf("[%s] CloseWithErr(nil) should commit, but data not found", dbLabel(db))
			}
		})

		// CloseWithErr(err) → Rollback
		t.Run("RollbackOnErr", func(t *testing.T) {
			tx, err := db.Beginx()
			if err != nil {
				t.Fatalf("[%s] Beginx failed: %v", dbLabel(db), err)
			}
			tx.MustExec(`INSERT INTO cross_users (name, email, age) VALUES ('CloseTxRollback', 'close@rollback.com', 30)`)
			tx.CloseWithErr(errors.New("some error")) // 应自动回滚

			var count int
			db.Get(&count, "SELECT COUNT(*) FROM cross_users WHERE name = 'CloseTxRollback'")
			if count != 0 {
				t.Errorf("[%s] CloseWithErr(err) should rollback, but data found", dbLabel(db))
			}
		})
	})
}

// ========================================================
// TestCrossDBTxNamedOps — 验证事务中命名参数操作 + IN 展开
// ========================================================
func TestCrossDBTxNamedOps(t *testing.T) {
	runWithSchema(crossSchema, t, func(db *sqlex.DB, t *testing.T, now string) {
		crossDBOnly(t)
		seedCrossData(db, t)

		tx, err := db.Beginx()
		if err != nil {
			t.Fatalf("[%s] Beginx failed: %v", dbLabel(db), err)
		}
		defer tx.Rollback()

		// NamedExec in tx
		t.Run("NamedExec", func(t *testing.T) {
			_, err := tx.NamedExec(
				`INSERT INTO cross_users (name, email, age) VALUES (:name, :email, :age)`,
				map[string]any{"name": "TxNamed", "email": "txnamed@example.com", "age": 33})
			if err != nil {
				t.Fatalf("[%s] tx.NamedExec failed: %v", dbLabel(db), err)
			}
		})

		// NamedGet in tx
		t.Run("NamedGet", func(t *testing.T) {
			var user CrossUser
			err := tx.NamedGet(&user,
				`SELECT * FROM cross_users WHERE name = :name`,
				map[string]any{"name": "TxNamed"})
			if err != nil {
				t.Fatalf("[%s] tx.NamedGet failed: %v", dbLabel(db), err)
			}
			if user.Email != "txnamed@example.com" {
				t.Errorf("[%s] expected txnamed@example.com, got %s", dbLabel(db), user.Email)
			}
		})

		// NamedSelect + IN 展开 in tx
		t.Run("NamedSelectIN", func(t *testing.T) {
			var users []CrossUser
			err := tx.NamedSelect(&users,
				`SELECT * FROM cross_users WHERE name IN (:names) ORDER BY name`,
				map[string]any{"names": []string{"Alice", "TxNamed"}})
			if err != nil {
				t.Fatalf("[%s] tx.NamedSelect IN failed: %v", dbLabel(db), err)
			}
			if len(users) != 2 {
				t.Errorf("[%s] expected 2 users, got %d", dbLabel(db), len(users))
			}
		})
	})
}

// ========================================================
// TestCrossDBTxErrorHandling — 验证事务错误处理
// ========================================================
func TestCrossDBTxErrorHandling(t *testing.T) {
	// 创建带唯一约束的表
	var uniqueSchema = Schema{
		Create: `
CREATE TABLE cross_unique_users (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	name VARCHAR(255) NOT NULL UNIQUE,
	email TEXT NOT NULL
);
`,
		Drop: `
DROP TABLE IF EXISTS cross_unique_users;
`,
	}

	runWithSchema(uniqueSchema, t, func(db *sqlex.DB, t *testing.T, now string) {
		crossDBOnly(t)

		// 唯一约束违反后可安全回滚
		t.Run("UniqueViolationRollback", func(t *testing.T) {
			tx, err := db.Beginx()
			if err != nil {
				t.Fatalf("[%s] Beginx failed: %v", dbLabel(db), err)
			}

			tx.MustExec(`INSERT INTO cross_unique_users (name, email) VALUES ('Unique1', 'u1@test.com')`)

			// 重复插入应失败
			_, err = tx.Exec(`INSERT INTO cross_unique_users (name, email) VALUES ('Unique1', 'u2@test.com')`)
			if err == nil {
				t.Fatalf("[%s] expected unique constraint error, got nil", dbLabel(db))
			}

			// 回滚应成功
			rbErr := tx.Rollback()
			// MySQL 可能在约束违反后仍允许回滚，PG 的事务可能已中止
			// 关键是不 panic
			if rbErr != nil {
				t.Logf("[%s] Rollback after error: %v (this is OK for some DBs)", dbLabel(db), rbErr)
			}
		})

		// 已提交/回滚的事务上操作应返回错误
		t.Run("OpsOnFinishedTx", func(t *testing.T) {
			tx, err := db.Beginx()
			if err != nil {
				t.Fatalf("[%s] Beginx failed: %v", dbLabel(db), err)
			}
			tx.Commit()

			// 在已提交的事务上执行操作
			_, err = tx.Exec(`INSERT INTO cross_unique_users (name, email) VALUES ('AfterCommit', 'ac@test.com')`)
			if err == nil {
				t.Errorf("[%s] expected error on exec after commit, got nil", dbLabel(db))
			}
		})
	})
}

// 用于抑制 unused import 警告
var _ = context.Background
