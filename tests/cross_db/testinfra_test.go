// testinfra_test.go — Cross-DB integration test infrastructure
//
// Contains Schema type, database connection management, RunWithSchema framework.
// Since this file belongs to the cross_db_test package (external test package),
// it can safely import sqlex without creating a circular dependency with the sqlex package itself.
package cross_db_test

import (
	"fmt"
	"os"
	"strings"
	"testing"

	_ "github.com/go-sql-driver/mysql"  // register mysql driver
	_ "github.com/lib/pq"               // register postgres driver
	_ "github.com/mattn/go-sqlite3"     // register sqlite3 driver
	_ "github.com/microsoft/go-mssqldb" // register sqlserver driver

	sqlex "github.com/go-sqlex/sqlex"
	"github.com/go-sqlex/sqlex/testutil"
)

// ---------- Schema DDL templates ----------

// Schema defines DDL templates for creating/dropping tables (based on SQLite syntax, auto-converted to each DB dialect)
type Schema struct {
	Create string
	Drop   string
}

// Postgres returns PostgreSQL DDL and now() function
func (s Schema) Postgres() (string, string, string) {
	c := strings.Replace(s.Create, `INTEGER PRIMARY KEY AUTOINCREMENT`, `SERIAL PRIMARY KEY`, -1)
	return c, s.Drop, `now()`
}

// MySQL returns MySQL DDL and now() function
func (s Schema) MySQL() (string, string, string) {
	c := strings.Replace(s.Create, `"`, "`", -1)
	c = strings.Replace(c, `AUTOINCREMENT`, `AUTO_INCREMENT`, -1)
	return c, s.Drop, `now()`
}

// Sqlite3 returns SQLite3 DDL and CURRENT_TIMESTAMP
func (s Schema) Sqlite3() (string, string, string) {
	return strings.Replace(s.Create, `now()`, `CURRENT_TIMESTAMP`, -1), s.Drop, `CURRENT_TIMESTAMP`
}

// SQLServer returns SQL Server DDL and GETDATE() function
func (s Schema) SQLServer() (string, string, string) {
	c := s.Create
	// AUTOINCREMENT → IDENTITY(1,1) (case-insensitive)
	c = strings.Replace(c, `INTEGER PRIMARY KEY AUTOINCREMENT`, `INT IDENTITY(1,1) PRIMARY KEY`, -1)
	c = strings.Replace(c, `integer PRIMARY KEY AUTOINCREMENT`, `INT IDENTITY(1,1) PRIMARY KEY`, -1)
	// Double-quoted identifiers → remove
	c = strings.Replace(c, `"`, ``, -1)
	// CURRENT_TIMESTAMP → GETDATE() (process first to avoid TIMESTAMP→DATETIME breaking it)
	c = strings.Replace(c, `CURRENT_TIMESTAMP`, `GETDATE()`, -1)
	// TIMESTAMP type → DATETIME (SQL Server's TIMESTAMP is rowversion)
	c = strings.Replace(c, `TIMESTAMP`, `DATETIME`, -1)
	// now() → GETDATE()
	c = strings.Replace(c, `now()`, `GETDATE()`, -1)
	// text/TEXT → VARCHAR(MAX) (use VARCHAR instead of NVARCHAR to avoid go-mssqldb returning UTF-16 BOM for NVARCHAR columns)
	c = strings.Replace(c, ` TEXT`, ` VARCHAR(MAX)`, -1)
	c = strings.Replace(c, ` text`, ` VARCHAR(MAX)`, -1)
	// REAL → FLOAT
	c = strings.Replace(c, ` REAL`, ` FLOAT`, -1)
	// integer/INTEGER → INT
	c = strings.Replace(c, `INTEGER`, `INT`, -1)
	c = strings.Replace(c, `integer`, `INT`, -1)

	d := strings.Replace(s.Drop, `"`, ``, -1)
	return c, d, `GETDATE()`
}

// ---------- Database connection management ----------

var (
	isTestPostgres  = true
	isTestMysql     = true
	isTestSqlite    = true
	isTestSqlserver = true

	pgdb    *sqlex.DB
	mysqldb *sqlex.DB
	sldb    *sqlex.DB
	msdb    *sqlex.DB
)

func init() {
	testutil.LoadTestEnv()
	connectAll()
}

func connectAll() {
	var err error

	pgdsn := os.Getenv("SQLX_POSTGRES_DSN")
	mydsn := os.Getenv("SQLX_MYSQL_DSN")
	sqdsn := os.Getenv("SQLX_SQLITE_DSN")
	msdsn := os.Getenv("SQLX_SQLSERVER_DSN")

	// Empty string means not configured, skip that DB; explicitly set to "skip" also skips
	isTestPostgres = pgdsn != "" && pgdsn != "skip"
	isTestMysql = mydsn != "" && mydsn != "skip"
	isTestSqlite = sqdsn != "skip"
	isTestSqlserver = msdsn != "" && msdsn != "skip"

	// SQLite defaults to in-memory database when not configured
	if sqdsn == "" {
		sqdsn = ":memory:"
	}

	// Only append parseTime when DSN is non-empty and doesn't already contain it
	if mydsn != "" && !strings.Contains(mydsn, "parseTime=true") {
		if strings.Contains(mydsn, "?") {
			mydsn += "&parseTime=true"
		} else {
			mydsn += "?parseTime=true"
		}
	}

	if isTestPostgres {
		pgdb, err = sqlex.Connect("postgres", pgdsn)
		if err != nil {
			fmt.Printf("Disabling PG tests:\n    %v\n", err)
			isTestPostgres = false
		}
	} else {
		fmt.Println("Disabling Postgres tests.")
	}

	if isTestMysql {
		mysqldb, err = sqlex.Connect("mysql", mydsn)
		if err != nil {
			fmt.Printf("Disabling MySQL tests:\n    %v\n", err)
			isTestMysql = false
		}
	} else {
		fmt.Println("Disabling MySQL tests.")
	}

	if isTestSqlite {
		sldb, err = sqlex.Connect("sqlite3", sqdsn)
		if err != nil {
			fmt.Printf("Disabling SQLite:\n    %v\n", err)
			isTestSqlite = false
		}
	} else {
		fmt.Println("Disabling SQLite tests.")
	}

	if isTestSqlserver {
		msdb, err = sqlex.Connect("sqlserver", msdsn)
		if err != nil {
			fmt.Printf("Disabling SQL Server tests:\n    %v\n", err)
			isTestSqlserver = false
		}
	} else {
		fmt.Println("Disabling SQL Server tests.")
	}
}

// ---------- MultiExec & RunWithSchema ----------

// multiExec executes multiple SQL statements separated by semicolons
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

// runWithSchema runs tests on all available databases: auto-create table → run test → cleanup
// Cross-DB tests run on MySQL, PostgreSQL, and SQL Server (not SQLite)
func runWithSchema(schema Schema, t *testing.T, test func(db *sqlex.DB, t *testing.T, now string)) {
	runner := func(db *sqlex.DB, t *testing.T, create, drop, now string) {
		defer func() {
			multiExec(db, drop)
		}()

		// Clean up residual tables first (previous test may have been interrupted), then create, ensuring idempotency
		multiExec(db, drop)
		multiExec(db, create)
		test(db, t, now)
	}

	if isTestPostgres {
		create, drop, now := schema.Postgres()
		runner(pgdb, t, create, drop, now)
	}
	if isTestMysql {
		create, drop, now := schema.MySQL()
		runner(mysqldb, t, create, drop, now)
	}
	if isTestSqlserver {
		create, drop, now := schema.SQLServer()
		runner(msdb, t, create, drop, now)
	}
}
