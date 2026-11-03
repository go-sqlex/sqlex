// types_null_test.go — SQL NULL semantics for the types package, verified against real databases.
package cross_db_test

import (
	"database/sql"
	"testing"

	sqlex "github.com/go-sqlex/sqlex"
	"github.com/go-sqlex/sqlex/types"
)

// nullTypesSchema provides two nullable text columns to exercise SQL NULL round-trips.
var nullTypesSchema = Schema{
	Create: `
CREATE TABLE cross_null_types (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	doc TEXT,
	payload TEXT
);
`,
	Drop: `
DROP TABLE IF EXISTS cross_null_types;
`,
}

// TestCrossDBNullRoundTrip verifies SQL NULL handling end to end:
//   - #16: SQL NULL scanned into types.JSONText yields the "null" literal, not empty bytes.
//   - #20: types.GzippedText(nil) is written as SQL NULL, not a 20-byte empty gzip stream.
func TestCrossDBNullRoundTrip(t *testing.T) {
	runWithSchema(nullTypesSchema, t, func(db *sqlex.DB, t *testing.T, now string) {
		crossDBOnly(t)

		// #16: read SQL NULL into JSONText.
		if _, err := db.Exec(`INSERT INTO cross_null_types (doc, payload) VALUES (NULL, NULL)`); err != nil {
			t.Fatalf("[%s] insert NULL row failed: %v", dbLabel(db), err)
		}
		var id int64
		if err := db.Get(&id, `SELECT MAX(id) FROM cross_null_types`); err != nil {
			t.Fatalf("[%s] fetch id failed: %v", dbLabel(db), err)
		}

		var doc types.JSONText
		if err := db.Get(&doc, `SELECT doc FROM cross_null_types WHERE id = ?`, id); err != nil {
			t.Fatalf("[%s] scan JSONText failed: %v", dbLabel(db), err)
		}
		if string(doc) != "null" {
			t.Errorf("[%s] SQL NULL scanned into JSONText = %q, want %q", dbLabel(db), string(doc), "null")
		}

		// #20: write GzippedText(nil) and confirm the column is SQL NULL.
		if _, err := db.Exec(`INSERT INTO cross_null_types (doc, payload) VALUES (?, ?)`, "null", types.GzippedText(nil)); err != nil {
			t.Fatalf("[%s] insert nil GzippedText failed: %v", dbLabel(db), err)
		}
		var nilID int64
		if err := db.Get(&nilID, `SELECT MAX(id) FROM cross_null_types`); err != nil {
			t.Fatalf("[%s] fetch id failed: %v", dbLabel(db), err)
		}

		var payload sql.NullString
		if err := db.Get(&payload, `SELECT payload FROM cross_null_types WHERE id = ?`, nilID); err != nil {
			t.Fatalf("[%s] scan payload failed: %v", dbLabel(db), err)
		}
		if payload.Valid {
			t.Errorf("[%s] GzippedText(nil) persisted as non-NULL (%d bytes), want SQL NULL", dbLabel(db), len(payload.String))
		}
	})
}
