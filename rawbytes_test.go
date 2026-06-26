package sqlex

import (
	"bytes"
	"database/sql"
	"strings"
	"testing"
)

// TestScanAll_RawBytes_Corruption reproduces sqlx #931:
// Select() with []sql.RawBytes destination corrupts data because RawBytes
// references the driver's internal buffer, which is reused on each Next() call.
//
// Requires PostgreSQL (bytea type + lib/pq driver behavior).
func TestScanAll_RawBytes_Corruption(t *testing.T) {
	if !TestPostgres {
		t.Skip("PostgreSQL not available")
	}

	pgdb.MustExec(`DROP TABLE IF EXISTS rawbytes_test`)
	pgdb.MustExec(`CREATE TABLE rawbytes_test (data bytea)`)
	defer pgdb.MustExec(`DROP TABLE rawbytes_test`)

	pgdb.MustExec(`INSERT INTO rawbytes_test VALUES ('\xdead'), ('\xbeef'), ('\xdeadbeef')`)

	// []sql.RawBytes — RawBytes references driver buffer, corrupted by Select
	t.Run("slice_of_RawBytes_rejected", func(t *testing.T) {
		var result []sql.RawBytes
		err := pgdb.Select(&result, `SELECT data FROM rawbytes_test WHERE $1=1`, 1)
		if err == nil {
			t.Fatal("expected error rejecting sql.RawBytes in Select, got nil")
		}
		if !strings.Contains(err.Error(), "RawBytes") {
			t.Errorf("error should mention RawBytes: got %v", err)
		}
	})

	// struct with RawBytes field — same corruption, should be rejected
	t.Run("struct_with_RawBytes_field_rejected", func(t *testing.T) {
		type RawRow struct {
			Data sql.RawBytes `db:"data"`
		}
		var result []RawRow
		err := pgdb.Select(&result, `SELECT data FROM rawbytes_test WHERE $1=1`, 1)
		if err == nil {
			t.Fatal("expected error rejecting sql.RawBytes in struct field, got nil")
		}
		if !strings.Contains(err.Error(), "RawBytes") {
			t.Errorf("error should mention RawBytes: got %v", err)
		}
	})

	// Control: [][]byte works correctly (driver copies data for []byte)
	t.Run("slice_of_byte_slice_works", func(t *testing.T) {
		var result [][]byte
		err := pgdb.Select(&result, `SELECT data FROM rawbytes_test`)
		if err != nil {
			t.Fatalf("Select failed: %v", err)
		}
		expected := [][]byte{{0xde, 0xad}, {0xbe, 0xef}, {0xde, 0xad, 0xbe, 0xef}}
		for i, r := range result {
			if !bytes.Equal(r, expected[i]) {
				t.Errorf("row %d: expected %x, got %x", i, expected[i], r)
			}
		}
	})
}
