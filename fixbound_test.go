package sqlex

import (
	"testing"
)

// TestFixBound_ValuesExpansion reproduces sqlx issue #898/#694/#772:
// fixBound uses valuesReg = `\)\s*(?i)VALUES\s*\(` which requires a `)` before VALUES.
// This breaks batch expansion for:
//   - INSERT INTO t VALUES (:a, :b)          — no column list, VALUES preceded by table name
//   - UPDATE ... FROM (VALUES (:a, :b))       — PG syntax, VALUES preceded by (
//
// These tests expect CORRECT behavior (expansion). They will FAIL on current code,
// confirming the bug exists.
func TestFixBound_ValuesExpansion(t *testing.T) {
	type User struct {
		Name string `db:"name"`
		Age  int    `db:"age"`
	}

	users := []User{
		{Name: "alice", Age: 30},
		{Name: "bob", Age: 25},
	}

	// === Control: INSERT with column list — works correctly ===
	t.Run("insert_with_column_names_expands", func(t *testing.T) {
		q, args, err := Named(`INSERT INTO t (name, age) VALUES (:name, :age)`, users)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		want := `INSERT INTO t (name, age) VALUES (?, ?),(?, ?)`
		if q != want {
			t.Errorf("query mismatch:\n  got =%q\n  want=%q", q, want)
		}
		if len(args) != 4 {
			t.Errorf("args should be 4 (2 users * 2 fields): got len=%d", len(args))
		}
	})

	// === Bug: INSERT without column list — fixBound does not expand ===
	t.Run("insert_without_column_names_should_expand", func(t *testing.T) {
		q, args, err := Named(`INSERT INTO t VALUES (:name, :age)`, users)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		want := `INSERT INTO t VALUES (?, ?),(?, ?)`
		if q != want {
			t.Errorf("[BUG] query not expanded (VALUES preceded by table name, not ):\n  got =%q\n  want=%q", q, want)
		}
		if len(args) != 4 {
			t.Errorf("[BUG] args should be 4: got len=%d", len(args))
		}
	})

	// === Bug: PG UPDATE ... FROM (VALUES ...) — fixBound does not expand ===
	t.Run("pg_update_from_values_should_expand", func(t *testing.T) {
		type PriceUpdate struct {
			ID    int     `db:"id"`
			Price float64 `db:"new_price"`
		}

		updates := []PriceUpdate{
			{ID: 1, Price: 1.20},
			{ID: 2, Price: 0.55},
			{ID: 3, Price: 0.75},
		}

		q, args, err := Named(
			`UPDATE products SET price = v.new_price FROM (VALUES (:id, :new_price)) AS v(id, new_price) WHERE products.id = v.id`,
			updates,
		)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		want := `UPDATE products SET price = v.new_price FROM (VALUES (?, ?),(?, ?),(?, ?)) AS v(id, new_price) WHERE products.id = v.id`
		if q != want {
			t.Errorf("[BUG] query not expanded (VALUES preceded by (, not ):\n  got =%q\n  want=%q", q, want)
		}
		if len(args) != 6 {
			t.Errorf("[BUG] args should be 6 (3 updates * 2 fields): got len=%d", len(args))
		}
	})

	// === Regression: string literal containing ") VALUES (" should not be misidentified ===
	t.Run("values_in_string_literal_not_misidentified", func(t *testing.T) {
		// The SQL contains a string literal with ") VALUES (" inside.
		// After the fix (lexer-based), the lexer must skip the string literal and find
		// the real VALUES clause. With the old regex, this could be misidentified.
		q, args, err := Named(
			`INSERT INTO logs (msg, extra) VALUES (:name, 'a) VALUES (b')`,
			[]User{
				{Name: "hello", Age: 0},
				{Name: "world", Age: 1},
			},
		)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		// Should expand the real VALUES clause, not the one inside the string literal
		want := `INSERT INTO logs (msg, extra) VALUES (?, 'a) VALUES (b'),(?, 'a) VALUES (b')`
		if q != want {
			t.Errorf("query mismatch (string literal VALUES should be skipped):\n  got =%q\n  want=%q", q, want)
		}
		// Only :name is a named param; 'a) VALUES (b' is a string literal constant.
		// 2 users * 1 param = 2 args.
		if len(args) != 2 {
			t.Errorf("args should be 2 (2 users * 1 named param): got len=%d", len(args))
		}
	})
}

func TestFindValuesKeyword_Boundary(t *testing.T) {
	// Preceding boundary: ident before VALUES → not a keyword
	if pos := findValuesKeyword(`INSERT INTO table_values (a) VALUES (:a)`); pos < 0 {
		t.Fatal("should find VALUES after table_values")
	}
	// Following boundary: ident after VALUES → not a keyword
	if pos := findValuesKeyword(`INSERT INTO t (a) values_list (:a)`); pos >= 0 {
		t.Errorf("values_list should not match, got pos=%d", pos)
	}
	// Case insensitive
	if pos := findValuesKeyword(`INSERT INTO t (a) values (:a)`); pos < 0 {
		t.Fatal("lowercase values should be found")
	}
	// No VALUES at all
	if pos := findValuesKeyword(`SELECT * FROM t`); pos >= 0 {
		t.Errorf("should not find VALUES in SELECT, got pos=%d", pos)
	}
}
