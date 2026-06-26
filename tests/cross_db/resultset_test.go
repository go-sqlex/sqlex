package cross_db_test

import (
	"testing"
)

// TestCrossDBRowsNextResultSet reproduces sqlx #857:
// Rows.NextResultSet() is not overridden — the StructScan cache (started/fields/values)
// is not invalidated, causing the second result set to use stale field mappings.
//
// SQL Server supports multi-statement queries natively (no multiStatements flag needed):
//
//	SELECT 1 AS a; SELECT 2 AS b
//
// returns two result sets with different column structures.
func TestCrossDBRowsNextResultSet(t *testing.T) {
	if !isTestSqlserver {
		t.Skip("SQL Server not available")
	}

	rows, err := msdb.Queryx("SELECT 1 AS a; SELECT 2 AS b, 'extra' AS c")
	if err != nil {
		t.Fatalf("[SQLSERVER] Queryx failed: %v", err)
	}
	defer rows.Close()

	// First result set: 1 column — struct{ A int }
	type ResultA struct{ A int }
	for rows.Next() {
		var r ResultA
		if err := rows.StructScan(&r); err != nil {
			t.Fatalf("[SQLSERVER] first result set StructScan failed: %v", err)
		}
		if r.A != 1 {
			t.Errorf("[SQLSERVER] first result set: expected A=1, got %d", r.A)
		}
	}

	// Switch to second result set
	if !rows.NextResultSet() {
		t.Fatal("[SQLSERVER] expected second result set, got none")
	}

	// Second result set: 2 columns (b, c) — different structure
	// Before fix: stale cache from first result set (1 column) causes Scan argument count mismatch
	type ResultB struct {
		B int
		C string
	}
	for rows.Next() {
		var r ResultB
		if err := rows.StructScan(&r); err != nil {
			t.Fatalf("[SQLSERVER] second result set StructScan failed (cache not reset?): %v", err)
		}
		if r.B != 2 {
			t.Errorf("[SQLSERVER] second result set: expected B=2, got %d", r.B)
		}
		if r.C != "extra" {
			t.Errorf("[SQLSERVER] second result set: expected C='extra', got %q", r.C)
		}
	}
}
