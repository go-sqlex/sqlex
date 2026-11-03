// bitbool_test.go — cross-database integration tests for types.BitBool
package cross_db_test

import (
	"testing"

	sqlex "github.com/go-sqlex/sqlex"
	"github.com/go-sqlex/sqlex/types"
)

// TestCrossDBBitBoolScan — 验证 BitBool 能正确读取 BIT(1) 列：
// MySQL 返回 []byte{1}，PostgreSQL 返回文本 "1"
func TestCrossDBBitBoolScan(t *testing.T) {
	runWithSchema(Schema{Drop: "DROP TABLE IF EXISTS cross_bitbool"}, t, func(db *sqlex.DB, t *testing.T, now string) {
		// BIT(1) is only tested on MySQL and PostgreSQL. Use return rather than t.Skip,
		// which would end the test for the remaining databases sharing this t.
		if !isMySQL(db) && !isPostgres(db) {
			return
		}
		db.MustExec("CREATE TABLE cross_bitbool (id INTEGER, b BIT(1))")
		db.MustExec("INSERT INTO cross_bitbool (id, b) VALUES (1, B'1'), (2, B'0')")

		var got []types.BitBool
		if err := db.Select(&got, "SELECT b FROM cross_bitbool ORDER BY id"); err != nil {
			t.Fatalf("[%s] Select BitBool failed: %v", dbLabel(db), err)
		}
		if len(got) != 2 || !got[0] || got[1] {
			t.Errorf("[%s] expected [true false], got %v", dbLabel(db), got)
		}

		// A nullable column maps to a *BitBool struct field, which database/sql
		// sets to nil for NULL — the documented replacement for treating NULL as
		// false.
		db.MustExec("INSERT INTO cross_bitbool (id, b) VALUES (3, NULL)")
		type bitRow struct {
			ID int            `db:"id"`
			B  *types.BitBool `db:"b"`
		}
		var rows []bitRow
		if err := db.Select(&rows, "SELECT id, b FROM cross_bitbool ORDER BY id"); err != nil {
			t.Fatalf("[%s] Select rows with *BitBool failed: %v", dbLabel(db), err)
		}
		if len(rows) != 3 || rows[0].B == nil || !*rows[0].B || rows[1].B == nil || *rows[1].B || rows[2].B != nil {
			t.Errorf("[%s] expected B = [true false nil], got %+v", dbLabel(db), rows)
		}

		// Scanning NULL into a plain BitBool is an error, as it is for bool.
		var plain types.BitBool
		if err := db.Get(&plain, "SELECT b FROM cross_bitbool WHERE id = 3"); err == nil {
			t.Errorf("[%s] scanning NULL into BitBool should fail, got %v", dbLabel(db), plain)
		}
	})
}
