package sqlex

import (
	"bytes"
	"database/sql"
	"reflect"
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

	// struct with *sql.RawBytes (pointer) field — should also be rejected
	t.Run("struct_with_ptr_to_RawBytes_rejected", func(t *testing.T) {
		type PtrRawRow struct {
			Data *sql.RawBytes `db:"data"`
		}
		var result []PtrRawRow
		err := pgdb.Select(&result, `SELECT data FROM rawbytes_test WHERE $1=1`, 1)
		if err == nil {
			t.Fatal("expected error rejecting *sql.RawBytes in struct field, got nil")
		}
		if !strings.Contains(err.Error(), "RawBytes") {
			t.Errorf("error should mention RawBytes: got %v", err)
		}
	})

	// nested struct with RawBytes — should be rejected recursively
	t.Run("nested_struct_with_RawBytes_rejected", func(t *testing.T) {
		type Inner struct {
			Data sql.RawBytes `db:"data"`
		}
		type Outer struct {
			Inner `db:""`
		}
		var result []Outer
		err := pgdb.Select(&result, `SELECT data FROM rawbytes_test WHERE $1=1`, 1)
		if err == nil {
			t.Fatal("expected error rejecting nested RawBytes, got nil")
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

// TestContainsRawBytes is a database-free unit test covering all branches of
// containsRawBytes, in particular the skip of unexported non-embedded fields
// (the continue path) which the PG-dependent integration test above cannot
// reliably exercise in CI.
func TestContainsRawBytes(t *testing.T) {
	type unexportedRaw struct {
		data sql.RawBytes // unexported, must be skipped
		Name string       // exported, not RawBytes
	}
	type withRawField struct {
		Data sql.RawBytes
	}
	type withPtrRawField struct {
		Data *sql.RawBytes
	}
	type nestedRaw struct {
		Inner struct {
			Data sql.RawBytes
		}
	}
	type cleanStruct struct {
		ID   int64
		Name string
	}
	type embeddedUnexportedRaw struct {
		data sql.RawBytes // unexported, must be skipped even though it's the only field
	}

	tests := []struct {
		name string
		v    any
		want bool
	}{
		{"raw bytes directly", sql.RawBytes{}, true},
		{"pointer to raw bytes", &sql.RawBytes{}, true},
		{"byte slice", []byte{}, false},
		{"string", "", false},
		{"int", 0, false},
		{"struct with exported raw field", withRawField{}, true},
		{"struct with ptr raw field", withPtrRawField{}, true},
		{"nested struct with raw", nestedRaw{}, true},
		{"clean struct no raw bytes", cleanStruct{}, false},
		{"unexported raw field skipped", unexportedRaw{}, false},
		{"only unexported raw field skipped", embeddedUnexportedRaw{}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := containsRawBytes(reflect.TypeOf(tt.v))
			if got != tt.want {
				t.Errorf("containsRawBytes(%T) = %v, want %v", tt.v, got, tt.want)
			}
		})
	}
}
