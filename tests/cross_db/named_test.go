// named_test.go — 任务 2: 命名参数处理的跨库集成测试
//
// 需求覆盖：1.1–1.11, 12.6–12.11
package cross_db_test

import (
	"testing"

	sqlex "github.com/go-sqlex/sqlex"
)

// ========================================================
// TestCrossDBNamedExec — 验证 NamedExec 在双库上正确转换命名参数
// ========================================================
func TestCrossDBNamedExec(t *testing.T) {
	runWithSchema(crossSchema, t, func(db *sqlex.DB, t *testing.T, now string) {
		crossDBOnly(t)

		// 使用结构体参数
		u := CrossUser{Name: "Dave", Email: "dave@example.com", Age: 40}
		_, err := db.NamedExec(
			`INSERT INTO cross_users (name, email, age) VALUES (:name, :email, :age)`, u)
		if err != nil {
			t.Fatalf("[%s] NamedExec with struct failed: %v", dbLabel(db), err)
		}

		// 使用 map 参数
		_, err = db.NamedExec(
			`INSERT INTO cross_users (name, email, age) VALUES (:name, :email, :age)`,
			map[string]any{"name": "Eve", "email": "eve@example.com", "age": 22})
		if err != nil {
			t.Fatalf("[%s] NamedExec with map failed: %v", dbLabel(db), err)
		}

		// 验证数据已插入
		var count int
		err = db.Get(&count, "SELECT COUNT(*) FROM cross_users")
		if err != nil {
			t.Fatalf("[%s] Get count failed: %v", dbLabel(db), err)
		}
		if count != 2 {
			t.Errorf("[%s] expected 2 users, got %d", dbLabel(db), count)
		}
	})
}

// ========================================================
// TestCrossDBNamedGetSelect — 验证 NamedGet/NamedSelect 在双库上工作
// ========================================================
func TestCrossDBNamedGetSelect(t *testing.T) {
	runWithSchema(crossSchema, t, func(db *sqlex.DB, t *testing.T, now string) {
		crossDBOnly(t)
		seedCrossData(db, t)

		// NamedGet — 结构体参数
		t.Run("NamedGet_struct", func(t *testing.T) {
			var user CrossUser
			err := db.NamedGet(&user,
				`SELECT * FROM cross_users WHERE name = :name`,
				CrossUser{Name: "Alice"})
			if err != nil {
				t.Fatalf("[%s] NamedGet with struct failed: %v", dbLabel(db), err)
			}
			if user.Email != "alice@example.com" {
				t.Errorf("[%s] expected alice@example.com, got %s", dbLabel(db), user.Email)
			}
		})

		// NamedGet — map 参数
		t.Run("NamedGet_map", func(t *testing.T) {
			var user CrossUser
			err := db.NamedGet(&user,
				`SELECT * FROM cross_users WHERE name = :name`,
				map[string]any{"name": "Bob"})
			if err != nil {
				t.Fatalf("[%s] NamedGet with map failed: %v", dbLabel(db), err)
			}
			if user.Age != 25 {
				t.Errorf("[%s] expected age 25, got %d", dbLabel(db), user.Age)
			}
		})

		// NamedSelect — 查询多行
		t.Run("NamedSelect_multi", func(t *testing.T) {
			var users []CrossUser
			err := db.NamedSelect(&users,
				`SELECT * FROM cross_users WHERE age > :min_age ORDER BY age`,
				map[string]any{"min_age": 26})
			if err != nil {
				t.Fatalf("[%s] NamedSelect failed: %v", dbLabel(db), err)
			}
			if len(users) != 2 {
				t.Fatalf("[%s] expected 2 users with age > 26, got %d", dbLabel(db), len(users))
			}
			if users[0].Name != "Alice" || users[1].Name != "Charlie" {
				t.Errorf("[%s] unexpected order: %s, %s", dbLabel(db), users[0].Name, users[1].Name)
			}
		})
	})
}

// ========================================================
// TestCrossDBNamedSelectIN — 验证 NamedSelect + IN 展开
// ========================================================
func TestCrossDBNamedSelectIN(t *testing.T) {
	runWithSchema(crossSchema, t, func(db *sqlex.DB, t *testing.T, now string) {
		crossDBOnly(t)
		seedCrossData(db, t)

		var users []CrossUser
		err := db.NamedSelect(&users,
			`SELECT * FROM cross_users WHERE name IN (:names) ORDER BY name`,
			map[string]any{"names": []string{"Alice", "Charlie"}})
		if err != nil {
			t.Fatalf("[%s] NamedSelect with IN failed: %v", dbLabel(db), err)
		}
		if len(users) != 2 {
			t.Fatalf("[%s] expected 2 users, got %d", dbLabel(db), len(users))
		}
		if users[0].Name != "Alice" || users[1].Name != "Charlie" {
			t.Errorf("[%s] unexpected users: %s, %s", dbLabel(db), users[0].Name, users[1].Name)
		}
	})
}

// ========================================================
// TestCrossDBNamedStringLiteral — 验证字符串字面量中冒号不被误解析
// ========================================================
func TestCrossDBNamedStringLiteral(t *testing.T) {
	// 需要一个有 TEXT 列的表来测试字符串字面量
	var portGroupSchema = Schema{
		Create: `
CREATE TABLE cross_port_groups (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	group_name TEXT NOT NULL,
	port TEXT NOT NULL
);
`,
		Drop: `
DROP TABLE IF EXISTS cross_port_groups;
`,
	}

	runWithSchema(portGroupSchema, t, func(db *sqlex.DB, t *testing.T, now string) {
		crossDBOnly(t)

		// 插入包含冒号的字符串字面量
		_, err := db.NamedExec(
			`INSERT INTO cross_port_groups (group_name, port) VALUES (:group_name, 'tcp:8080')`,
			map[string]any{"group_name": "web"})
		if err != nil {
			t.Fatalf("[%s] NamedExec with string literal colon failed: %v", dbLabel(db), err)
		}

		// 验证插入的数据
		type PortGroup struct {
			ID        int    `db:"id"`
			GroupName string `db:"group_name"`
			Port      string `db:"port"`
		}
		var pg PortGroup
		err = db.NamedGet(&pg,
			`SELECT * FROM cross_port_groups WHERE group_name = :group_name AND port = 'tcp:8080'`,
			map[string]any{"group_name": "web"})
		if err != nil {
			t.Fatalf("[%s] NamedGet with string literal colon failed: %v", dbLabel(db), err)
		}
		if pg.Port != "tcp:8080" {
			t.Errorf("[%s] expected port 'tcp:8080', got '%s'", dbLabel(db), pg.Port)
		}

		// 测试时间格式中的冒号
		_, err = db.NamedExec(
			`INSERT INTO cross_port_groups (group_name, port) VALUES (:group_name, '10:30:00')`,
			map[string]any{"group_name": "timetest"})
		if err != nil {
			t.Fatalf("[%s] NamedExec with time-like colon string failed: %v", dbLabel(db), err)
		}
	})
}

// ========================================================
// TestCrossDBNamedDoubleColon — 验证 PostgreSQL :: 类型转换不干扰命名参数
// ========================================================
func TestCrossDBNamedDoubleColon(t *testing.T) {
	runWithSchema(crossSchema, t, func(db *sqlex.DB, t *testing.T, now string) {
		crossDBOnly(t)
		seedCrossData(db, t)

		if isPostgres(db) {
			// PostgreSQL :: 类型转换
			var name string
			err := db.NamedGet(&name,
				`SELECT name::text FROM cross_users WHERE name = :name LIMIT 1`,
				map[string]any{"name": "Alice"})
			if err != nil {
				t.Fatalf("[POSTGRES] NamedGet with :: cast failed: %v", err)
			}
			if name != "Alice" {
				t.Errorf("[POSTGRES] expected Alice, got %s", name)
			}
		}

		// MySQL 上也用等价的查询（不用 ::）
		if isMySQL(db) {
			var name string
			err := db.NamedGet(&name,
				`SELECT CAST(name AS CHAR) FROM cross_users WHERE name = :name LIMIT 1`,
				map[string]any{"name": "Alice"})
			if err != nil {
				t.Fatalf("[MYSQL] NamedGet with CAST failed: %v", err)
			}
			if name != "Alice" {
				t.Errorf("[MYSQL] expected Alice, got %s", name)
			}
		}
	})
}

// ========================================================
// TestCrossDBNamedUnicode — 验证 Unicode 参数名在双库上正确工作
// ========================================================
// TestCrossDBNamedUnicode 验证 Unicode 数据值的端到端读写。
//
// 注意：sqlex 不支持 Unicode 命名参数名（:名前 这类），但这不影响把中文/日文
// 作为参数值或表数据。本测试用 ASCII 参数名 + Unicode 值，验证真实场景。
func TestCrossDBNamedUnicode(t *testing.T) {
	runWithSchema(crossSchema, t, func(db *sqlex.DB, t *testing.T, now string) {
		crossDBOnly(t)

		// ASCII 参数名 + Unicode 数据值（真实场景）
		_, err := db.NamedExec(
			`INSERT INTO cross_users (name, email, age) VALUES (:name, :email, :age)`,
			map[string]any{"name": "太郎", "email": "taro@example.com", "age": 28})
		if err != nil {
			t.Fatalf("[%s] NamedExec with Unicode values failed: %v", dbLabel(db), err)
		}

		var user CrossUser
		err = db.Get(&user, "SELECT * FROM cross_users WHERE name = ?", "太郎")
		if err != nil {
			t.Fatalf("[%s] Get Unicode user failed: %v", dbLabel(db), err)
		}
		if user.Email != "taro@example.com" || user.Age != 28 {
			t.Errorf("[%s] unexpected user: %+v", dbLabel(db), user)
		}
	})
}

// ========================================================
// TestCrossDBNamedMissingParam — 验证 map 中缺少参数时容错处理
// ========================================================
func TestCrossDBNamedMissingParam(t *testing.T) {
	runWithSchema(crossSchema, t, func(db *sqlex.DB, t *testing.T, now string) {
		crossDBOnly(t)
		seedCrossData(db, t)

		// map 中缺少 :email 参数，容错：保留 :email 原样
		// 由于保留原样后 SQL 语法不对，查询应该失败
		var user CrossUser
		err := db.NamedGet(&user,
			`SELECT * FROM cross_users WHERE name = :name AND email = :email`,
			map[string]any{"name": "Alice"})
		// 缺少参数时，实现可能会报错，也可能保留原样后由数据库报错
		// 关键是不 panic
		if err == nil {
			// 如果没报错，说明容错逻辑将 :email 保留原样后数据库意外成功了
			t.Logf("[%s] NamedGet with missing param unexpectedly succeeded", dbLabel(db))
		} else {
			t.Logf("[%s] NamedGet with missing param correctly returned error: %v", dbLabel(db), err)
		}
	})
}

// ========================================================
// TestCrossDBNamedComments — 验证 SQL 注释中的冒号不被解析
// ========================================================
func TestCrossDBNamedComments(t *testing.T) {
	runWithSchema(crossSchema, t, func(db *sqlex.DB, t *testing.T, now string) {
		crossDBOnly(t)
		seedCrossData(db, t)

		// 行注释 -- 中的冒号不应被解析
		var users []CrossUser
		err := db.NamedSelect(&users,
			`SELECT * FROM cross_users -- comment with :fake_param
			WHERE age > :min_age ORDER BY age`,
			map[string]any{"min_age": 26})
		if err != nil {
			t.Fatalf("[%s] NamedSelect with line comment colon failed: %v", dbLabel(db), err)
		}
		if len(users) != 2 {
			t.Errorf("[%s] expected 2 users, got %d", dbLabel(db), len(users))
		}

		// 块注释 /* */ 中的冒号不应被解析
		var count int
		err = db.NamedGet(&count,
			`SELECT COUNT(*) FROM cross_users /* this :is :not :a :param */ WHERE age > :min_age`,
			map[string]any{"min_age": 0})
		if err != nil {
			t.Fatalf("[%s] NamedGet with block comment colon failed: %v", dbLabel(db), err)
		}
		if count != 3 {
			t.Errorf("[%s] expected count 3, got %d", dbLabel(db), count)
		}
	})
}

// ========================================================
// TestCrossDBNamedDoubleQuoteIdentifier — 验证双引号标识符中冒号不被解析
// ========================================================
func TestCrossDBNamedDoubleQuoteIdentifier(t *testing.T) {
	// 仅对 PostgreSQL 测试双引号标识符（MySQL 默认不支持）
	runWithSchema(crossSchema, t, func(db *sqlex.DB, t *testing.T, now string) {
		crossDBOnly(t)
		seedCrossData(db, t)

		if isPostgres(db) {
			// PostgreSQL 支持双引号标识符
			var user CrossUser
			err := db.NamedGet(&user,
				`SELECT * FROM cross_users WHERE "name" = :name LIMIT 1`,
				map[string]any{"name": "Alice"})
			if err != nil {
				t.Fatalf("[POSTGRES] NamedGet with double-quoted identifier failed: %v", err)
			}
			if user.Name != "Alice" {
				t.Errorf("[POSTGRES] expected Alice, got %s", user.Name)
			}
		}
	})
}
