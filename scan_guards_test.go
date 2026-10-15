package sqlex

import (
	"database/sql"
	"errors"
	"testing"
)

// TestRowsStructScan_NilPointer verifies StructScan returns an error on a typed
// nil pointer destination instead of panicking (#10 F-0001).
func TestRowsStructScan_NilPointer(t *testing.T) {
	r := Rows{}
	err := r.StructScan((*struct{ X int })(nil))
	if err == nil {
		t.Fatal("expected error for nil pointer destination, got nil")
	}
}

// TestRowColumns_NilRows verifies Columns/ColumnTypes return an error instead of
// panicking when the Row holds no rows and no deferred error (#10 F-0008).
func TestRowColumns_NilRows(t *testing.T) {
	var r Row
	if _, err := r.Columns(); !errors.Is(err, sql.ErrNoRows) {
		t.Errorf("Columns: expected sql.ErrNoRows, got %v", err)
	}
	if _, err := r.ColumnTypes(); !errors.Is(err, sql.ErrNoRows) {
		t.Errorf("ColumnTypes: expected sql.ErrNoRows, got %v", err)
	}
}
