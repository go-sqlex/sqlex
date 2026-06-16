// testinfra_test.go — PostgreSQL 专用集成测试基础设施
package pg_test

import (
	"fmt"
	"os"
	"strings"
	"testing"

	_ "github.com/lib/pq" // 注册 postgres 驱动

	sqlex "github.com/go-sqlex/sqlex"
	"github.com/go-sqlex/sqlex/testutil"
)

// ---------- Schema DDL 模板 ----------

// Schema 定义创建/删除表的 DDL 模板（可直接使用 PostgreSQL 语法）
type Schema struct {
	Create string
	Drop   string
}

// Postgres 返回 PostgreSQL 的 DDL 和 now() 函数
func (s Schema) Postgres() (string, string, string) {
	c := strings.Replace(s.Create, `INTEGER PRIMARY KEY AUTOINCREMENT`, `SERIAL PRIMARY KEY`, -1)
	return c, s.Drop, `now()`
}

// ---------- 数据库连接管理 ----------

var (
	isTestPostgres = true
	pgdb           *sqlex.DB
)

func init() {
	testutil.LoadTestEnv()
	connectPG()
}

func connectPG() {
	pgdsn := os.Getenv("SQLX_POSTGRES_DSN")
	isTestPostgres = pgdsn != "" && pgdsn != "skip"

	if isTestPostgres {
		var err error
		pgdb, err = sqlex.Connect("postgres", pgdsn)
		if err != nil {
			fmt.Printf("Disabling PG tests:\n    %v\n", err)
			isTestPostgres = false
		}
	} else {
		fmt.Println("Disabling Postgres tests.")
	}
}

// pgOnly 跳过非 PostgreSQL 环境
func pgOnly(t *testing.T) {
	t.Helper()
	if !isTestPostgres {
		t.Skip("PostgreSQL not available, skipping")
	}
}

// ---------- MultiExec ----------

// multiExec 执行用分号分隔的多条 SQL
func multiExec(e sqlex.Execer, query string) {
	stmts := strings.Split(query, ";\n")
	if len(strings.Trim(stmts[len(stmts)-1], " \n\t\r")) == 0 {
		stmts = stmts[:len(stmts)-1]
	}
	for _, s := range stmts {
		_, err := e.Exec(s)
		if err != nil {
			fmt.Println(err, s)
		}
	}
}
