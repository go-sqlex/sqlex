// json_test.go — cross-database integration tests for JsonValue[T] type
package cross_db_test

import (
	"context"
	"testing"

	sqlex "github.com/go-sqlex/sqlex"
	"github.com/go-sqlex/sqlex/types"
)

// crossJSONSchema 定义包含 JSON 列的跨库测试表
// MySQL 用 TEXT 列（兼容性更好），PostgreSQL 用 TEXT 或 JSONB
var crossJSONSchema = Schema{
	Create: `
CREATE TABLE cross_json_test (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	name TEXT NOT NULL,
	metadata TEXT
);
`,
	Drop: `
DROP TABLE IF EXISTS cross_json_test;
`,
}

// CrossJSONMetadata test struct for JSON columns
type CrossJSONMetadata struct {
	Version string `json:"version"`
	Author  string `json:"author"`
}

// CrossJSONRow test row struct
type CrossJSONRow struct {
	ID       int                                  `db:"id"`
	Name     string                               `db:"name"`
	Metadata types.JsonValue[CrossJSONMetadata] `db:"metadata"`
}

// ========================================================
// TestCrossDBJsonValueCRUD — 验证 JsonValue[T] 的写入、读取、更新
// ========================================================
func TestCrossDBJsonValueCRUD(t *testing.T) {
	runWithSchema(crossJSONSchema, t, func(db *sqlex.DB, t *testing.T, now string) {
		crossDBOnly(t)

		ctx := context.Background()

		// 写入
		meta := types.NewJsonValue(CrossJSONMetadata{Version: "1.0", Author: "Alice"})
		_, err := db.ExecContext(ctx,
			"INSERT INTO cross_json_test (name, metadata) VALUES (?, ?)",
			"test1", meta)
		if err != nil {
			t.Fatalf("[%s] Insert JsonValue failed: %v", dbLabel(db), err)
		}

		// 读取
		var row CrossJSONRow
		err = db.GetContext(ctx, &row, "SELECT * FROM cross_json_test WHERE name = ?", "test1")
		if err != nil {
			t.Fatalf("[%s] Get JsonValue failed: %v", dbLabel(db), err)
		}
		if !row.Metadata.Valid {
			t.Fatalf("[%s] expected Metadata.Valid = true", dbLabel(db))
		}
		if row.Metadata.Val.Version != "1.0" || row.Metadata.Val.Author != "Alice" {
			t.Errorf("[%s] unexpected metadata: %+v", dbLabel(db), row.Metadata.Val)
		}

		// 更新
		newMeta := types.NewJsonValue(CrossJSONMetadata{Version: "2.0", Author: "Bob"})
		_, err = db.ExecContext(ctx,
			"UPDATE cross_json_test SET metadata = ? WHERE name = ?",
			newMeta, "test1")
		if err != nil {
			t.Fatalf("[%s] Update JsonValue failed: %v", dbLabel(db), err)
		}

		// 验证更新
		err = db.GetContext(ctx, &row, "SELECT * FROM cross_json_test WHERE name = ?", "test1")
		if err != nil {
			t.Fatalf("[%s] Get updated JsonValue failed: %v", dbLabel(db), err)
		}
		if row.Metadata.Val.Version != "2.0" || row.Metadata.Val.Author != "Bob" {
			t.Errorf("[%s] unexpected updated metadata: %+v", dbLabel(db), row.Metadata.Val)
		}
	})
}

// ========================================================
// TestCrossDBJsonValueNull — 验证 NULL 值时 JsonValue.Valid 为 false
// ========================================================
func TestCrossDBJsonValueNull(t *testing.T) {
	runWithSchema(crossJSONSchema, t, func(db *sqlex.DB, t *testing.T, now string) {
		crossDBOnly(t)

		ctx := context.Background()

		// 插入 NULL metadata
		_, err := db.ExecContext(ctx,
			"INSERT INTO cross_json_test (name, metadata) VALUES (?, NULL)", "null_test")
		if err != nil {
			t.Fatalf("[%s] Insert NULL metadata failed: %v", dbLabel(db), err)
		}

		var row CrossJSONRow
		err = db.GetContext(ctx, &row, "SELECT * FROM cross_json_test WHERE name = ?", "null_test")
		if err != nil {
			t.Fatalf("[%s] Get NULL metadata failed: %v", dbLabel(db), err)
		}
		if row.Metadata.Valid {
			t.Errorf("[%s] expected Metadata.Valid = false for NULL, got true", dbLabel(db))
		}
	})
}

// ========================================================
// TestCrossDBJsonValueZeroVal — verify Val is zero when !Valid
// ========================================================
func TestCrossDBJsonValueZeroVal(t *testing.T) {
	runWithSchema(crossJSONSchema, t, func(db *sqlex.DB, t *testing.T, now string) {
		crossDBOnly(t)

		ctx := context.Background()

		// 插入有效值和 NULL 值
		meta := types.NewJsonValue(CrossJSONMetadata{Version: "3.0", Author: "Charlie"})
		db.ExecContext(ctx, "INSERT INTO cross_json_test (name, metadata) VALUES (?, ?)", "valid", meta)
		db.ExecContext(ctx, "INSERT INTO cross_json_test (name, metadata) VALUES (?, NULL)", "null_val")

		// Valid: Val field directly returns actual value
		var validRow CrossJSONRow
		db.GetContext(ctx, &validRow, "SELECT * FROM cross_json_test WHERE name = ?", "valid")
		if !validRow.Metadata.Valid || validRow.Metadata.Val.Version != "3.0" {
			t.Errorf("[%s] valid row should have Valid=true and correct Val", dbLabel(db))
		}

		// NULL: Val is zero value (guaranteed by Scan)
		var nullRow CrossJSONRow
		db.GetContext(ctx, &nullRow, "SELECT * FROM cross_json_test WHERE name = ?", "null_val")
		if nullRow.Metadata.Valid || nullRow.Metadata.Val.Version != "" {
			t.Errorf("[%s] null row should have Valid=false and zero-value Val", dbLabel(db))
		}
	})
}
