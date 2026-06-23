package sqlex

import (
	"database/sql/driver"
	"reflect"
	"strings"
	"testing"
)

// fakeValuer simulates driver.Valuer
type fakeValuer struct {
	val any
	err error
}

func (f fakeValuer) Value() (driver.Value, error) {
	return f.val, f.err
}

// TestIn_EdgeCases covers In function placeholder recognition across all SQL lexical elements.
// Goal: symmetric with Rebind / compileNamedQuery — ? inside SQL lexical regions is not
// recognized as a placeholder.
func TestIn_EdgeCases(t *testing.T) {
	cases := []struct {
		name      string
		query     string
		args      []any
		wantBound string
		wantArgs  []any
		wantErr   string // non-empty expects error (substring match)
	}{
		// ===== A. String literals =====
		{
			name:      "string literal ? preserved then IN expands",
			query:     `SELECT * FROM t WHERE name = 'test?' AND id IN (?)`,
			args:      []any{[]int{1, 2, 3}},
			wantBound: `SELECT * FROM t WHERE name = 'test?' AND id IN (?, ?, ?)`,
			wantArgs:  []any{1, 2, 3},
		},
		{
			name:      "multiple ? inside string",
			query:     `SELECT 'a?b?c' WHERE id IN (?)`,
			args:      []any{[]int{1, 2}},
			wantBound: `SELECT 'a?b?c' WHERE id IN (?, ?)`,
			wantArgs:  []any{1, 2},
		},
		{
			name:      "SQL escaped quote",
			query:     `SELECT 'O''Reilly?' WHERE id IN (?)`,
			args:      []any{[]int{1, 2}},
			wantBound: `SELECT 'O''Reilly?' WHERE id IN (?, ?)`,
			wantArgs:  []any{1, 2},
		},

		// ===== B. SQL comments =====
		{
			name:      "line comment ?",
			query:     "SELECT * FROM t -- WHERE id = ?\nWHERE x IN (?)",
			args:      []any{[]int{1, 2}},
			wantBound: "SELECT * FROM t -- WHERE id = ?\nWHERE x IN (?, ?)",
			wantArgs:  []any{1, 2},
		},
		{
			name:      "block comment ?",
			query:     `SELECT /* WHERE id = ? */ * FROM t WHERE x IN (?)`,
			args:      []any{[]int{1, 2}},
			wantBound: `SELECT /* WHERE id = ? */ * FROM t WHERE x IN (?, ?)`,
			wantArgs:  []any{1, 2},
		},

		// ===== C. Identifiers =====
		{
			name:      "PG double-quoted identifier with ?",
			query:     `SELECT "col?name" FROM t WHERE x IN (?)`,
			args:      []any{[]int{1, 2}},
			wantBound: `SELECT "col?name" FROM t WHERE x IN (?, ?)`,
			wantArgs:  []any{1, 2},
		},
		{
			name:      "MySQL backtick identifier with ?",
			query:     "SELECT `col?name` FROM t WHERE x IN (?)",
			args:      []any{[]int{1, 2}},
			wantBound: "SELECT `col?name` FROM t WHERE x IN (?, ?)",
			wantArgs:  []any{1, 2},
		},
		{
			name:      "SQL Server bracket identifier with ?",
			query:     `SELECT [col?name] FROM t WHERE x IN (?)`,
			args:      []any{[]int{1, 2}},
			wantBound: `SELECT [col?name] FROM t WHERE x IN (?, ?)`,
			wantArgs:  []any{1, 2},
		},
		{
			name:      "SQL Server bracket identifier with :name",
			query:     `SELECT [col:id] FROM t WHERE x IN (?)`,
			args:      []any{[]int{1, 2}},
			wantBound: `SELECT [col:id] FROM t WHERE x IN (?, ?)`,
			wantArgs:  []any{1, 2},
		},
		{
			name:      "SQL Server bracket escaped ]] with ?",
			query:     `SELECT [a]]?b] FROM t WHERE x IN (?)`,
			args:      []any{[]int{1, 2}},
			wantBound: `SELECT [a]]?b] FROM t WHERE x IN (?, ?)`,
			wantArgs:  []any{1, 2},
		},

		// ===== D. PG dollar-quoted =====
		{
			name:      "dollar quoting with ?",
			query:     `SELECT $$hello?$$ FROM t WHERE x IN (?)`,
			args:      []any{[]int{1, 2}},
			wantBound: `SELECT $$hello?$$ FROM t WHERE x IN (?, ?)`,
			wantArgs:  []any{1, 2},
		},
		{
			name:      "tagged dollar quoting",
			query:     `SELECT $tag$?$tag$ FROM t WHERE x IN (?)`,
			args:      []any{[]int{1, 2}},
			wantBound: `SELECT $tag$?$tag$ FROM t WHERE x IN (?, ?)`,
			wantArgs:  []any{1, 2},
		},

		// ===== E. Escapes =====
		{
			name:      "?? literal preserved",
			query:     `SELECT * WHERE x = ?? AND y IN (?)`,
			args:      []any{[]int{1, 2}},
			wantBound: `SELECT * WHERE x = ?? AND y IN (?, ?)`,
			wantArgs:  []any{1, 2},
		},
		{
			name:      "backslash escape",
			query:     `SELECT * WHERE x = \? AND y IN (?)`,
			args:      []any{[]int{1, 2}},
			wantBound: `SELECT * WHERE x = \? AND y IN (?, ?)`,
			wantArgs:  []any{1, 2},
		},

		// ===== F. driver.Valuer =====
		{
			name:      "Valuer non-slice",
			query:     `SELECT * WHERE x = ?`,
			args:      []any{fakeValuer{val: 42}},
			wantBound: `SELECT * WHERE x = ?`,
			wantArgs:  []any{fakeValuer{val: 42}}, // note: arg preserved as-is (driver calls Value internally)
		},
		{
			name:      "Valuer returns slice should expand",
			query:     `SELECT * WHERE x IN (?)`,
			args:      []any{fakeValuer{val: []int{1, 2, 3}}},
			wantBound: `SELECT * WHERE x IN (?, ?, ?)`,
			wantArgs:  []any{1, 2, 3},
		},
		{
			// Post-symmetry: Valuer returns slice but ? not in IN (?) context -> no expand, entire slice passed
			name:      "Valuer returns slice non-IN context no expand",
			query:     `SELECT * WHERE x = ?`,
			args:      []any{fakeValuer{val: []int{1, 2, 3}}},
			wantBound: `SELECT * WHERE x = ?`,
			wantArgs:  []any{[]int{1, 2, 3}}, // .Value() called, but entire slice as single value
		},

		// ===== G. Boundaries =====
		{
			name:    "empty slice error",
			query:   `SELECT * WHERE x IN (?)`,
			args:    []any{[]int{}},
			wantErr: "empty slice",
		},
		{
			name:      "single element slice",
			query:     `SELECT * WHERE x IN (?)`,
			args:      []any{[]int{42}},
			wantBound: `SELECT * WHERE x IN (?)`,
			wantArgs:  []any{42},
		},
		{
			name:      "slice with nil",
			query:     `SELECT * WHERE x IN (?)`,
			args:      []any{[]any{1, nil, 3}},
			wantBound: `SELECT * WHERE x IN (?, ?, ?)`,
			wantArgs:  []any{1, nil, 3},
		},
		{
			name:      "no slice fast path",
			query:     `SELECT * WHERE x = ? AND y = ?`,
			args:      []any{1, 2},
			wantBound: `SELECT * WHERE x = ? AND y = ?`, // as-is
			wantArgs:  []any{1, 2},
		},
		{
			name:      "multiple IN",
			query:     `SELECT * WHERE a IN (?) AND b IN (?)`,
			args:      []any{[]int{1, 2}, []string{"x", "y"}},
			wantBound: `SELECT * WHERE a IN (?, ?) AND b IN (?, ?)`,
			wantArgs:  []any{1, 2, "x", "y"},
		},
		{
			name:      "? and slice mixed",
			query:     `SELECT * WHERE x = ? AND id IN (?) AND y = ?`,
			args:      []any{100, []int{1, 2}, 200},
			wantBound: `SELECT * WHERE x = ? AND id IN (?, ?) AND y = ?`,
			wantArgs:  []any{100, 1, 2, 200},
		},
		{
			name:      "byte slice no expand",
			query:     `SELECT * WHERE blob = ?`,
			args:      []any{[]byte{1, 2, 3}},
			wantBound: `SELECT * WHERE blob = ?`,
			wantArgs:  []any{[]byte{1, 2, 3}},
		},

		// ===== H. Complex mixed =====
		{
			name:      "string comment backtick dollar quoting mixed",
			query:     "SELECT 'a?b', /* ? */ `c?d`, $$e?f$$ FROM t WHERE x IN (?)",
			args:      []any{[]int{10, 20}},
			wantBound: "SELECT 'a?b', /* ? */ `c?d`, $$e?f$$ FROM t WHERE x IN (?, ?)",
			wantArgs:  []any{10, 20},
		},

		// ===== I. IN list context awareness (only IN (?) expands) =====
		{
			name:      "IN no space tight parens",
			query:     `SELECT * WHERE x IN(?)`,
			args:      []any{[]int{1, 2}},
			wantBound: `SELECT * WHERE x IN(?, ?)`,
			wantArgs:  []any{1, 2},
		},
		{
			name:      "NOT IN expands",
			query:     `SELECT * WHERE x NOT IN (?)`,
			args:      []any{[]int{1, 2}},
			wantBound: `SELECT * WHERE x NOT IN (?, ?)`,
			wantArgs:  []any{1, 2},
		},
		{
			name:      "IN case insensitive",
			query:     `SELECT * WHERE x In (?)`,
			args:      []any{[]int{1, 2}},
			wantBound: `SELECT * WHERE x In (?, ?)`,
			wantArgs:  []any{1, 2},
		},
		{
			name:      "ANY no expand",
			query:     `SELECT * WHERE x = ANY(?)`,
			args:      []any{[]int{1, 2, 3}},
			wantBound: `SELECT * WHERE x = ANY(?)`,
			wantArgs:  []any{[]int{1, 2, 3}},
		},
		{
			name:      "ALL no expand",
			query:     `SELECT * WHERE x = ALL(?)`,
			args:      []any{[]int{1, 2, 3}},
			wantBound: `SELECT * WHERE x = ALL(?)`,
			wantArgs:  []any{[]int{1, 2, 3}},
		},
		{
			name:      "VALUES no expand",
			query:     `INSERT INTO t (col) VALUES (?)`,
			args:      []any{[]int{1, 2, 3}},
			wantBound: `INSERT INTO t (col) VALUES (?)`,
			wantArgs:  []any{[]int{1, 2, 3}},
		},
		{
			name:      "function call no expand",
			query:     `SELECT func(?)`,
			args:      []any{[]int{1, 2, 3}},
			wantBound: `SELECT func(?)`,
			wantArgs:  []any{[]int{1, 2, 3}},
		},
		{
			name:      "scalar subquery paren no expand",
			query:     `SELECT * WHERE x = (?)`,
			args:      []any{[]int{1, 2, 3}},
			wantBound: `SELECT * WHERE x = (?)`,
			wantArgs:  []any{[]int{1, 2, 3}},
		},
		{
			name:      "column name with in suffix not misidentified",
			query:     `SELECT * WHERE col_in (?)`,
			args:      []any{[]int{1, 2, 3}},
			wantBound: `SELECT * WHERE col_in (?)`,
			wantArgs:  []any{[]int{1, 2, 3}},
		},
		{
			name:      "qualified name t.in not misidentified",
			query:     `SELECT * WHERE t.in (?)`,
			args:      []any{[]int{1, 2, 3}},
			wantBound: `SELECT * WHERE t.in (?)`,
			wantArgs:  []any{[]int{1, 2, 3}},
		},
		{
			name:      "non-IN context AsList force expand",
			query:     `SELECT * WHERE x = ANY(?)`,
			args:      []any{AsList([]int{1, 2, 3})},
			wantBound: `SELECT * WHERE x = ANY(?, ?, ?)`,
			wantArgs:  []any{1, 2, 3},
		},
		{
			name:      "IN context AsValue suppress expand",
			query:     `SELECT * WHERE x IN (?)`,
			args:      []any{AsValue([]int{1, 2, 3})},
			wantBound: `SELECT * WHERE x IN (?)`,
			wantArgs:  []any{[]int{1, 2, 3}},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			gotBound, gotArgs, err := In(c.query, c.args...)

			if c.wantErr != "" {
				if err == nil {
					t.Fatalf("expected error containing %q, got nil", c.wantErr)
				}
				if !strings.Contains(err.Error(), c.wantErr) {
					t.Errorf("error = %q, want substring %q", err.Error(), c.wantErr)
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if gotBound != c.wantBound {
				t.Errorf("bound mismatch:\n  query=%q\n  got =%q\n  want=%q",
					c.query, gotBound, c.wantBound)
			}
			if !reflect.DeepEqual(gotArgs, c.wantArgs) {
				t.Errorf("args mismatch:\n  query=%q\n  got =%v\n  want=%v",
					c.query, gotArgs, c.wantArgs)
			}
		})
	}
}

// TestNeedsInRewrite_EdgeCases validates the needsInRewrite fast-path check.
// Real semantics: determines whether args need to go through the In path (slice expansion + AsValue/AsList unwrapping).
//
// Design: maintain zero-overhead fast path.
//   - Valuer is treated as a single value (even if it contains a slice internally)
//   - Slice pointers are not recognized (users should pass slices directly)
//   - Arrays (non-slices) are not expanded by In; needsInRewrite also doesn't recognize them
//   - AsValue / AsList wrappers always need In path for unwrapping (return true)
func TestNeedsInRewrite_EdgeCases(t *testing.T) {
	cases := []struct {
		name string
		args []any
		want bool
	}{
		{name: "empty args", args: []any{}, want: false},
		{name: "pure scalars", args: []any{1, "abc", true}, want: false},
		{name: "contains int slice", args: []any{1, []int{2, 3}}, want: true},
		{name: "byte slice ignored", args: []any{[]byte{1, 2, 3}}, want: false},
		{name: "Valuer non-slice", args: []any{fakeValuer{val: 1}}, want: false},
		// Valuer implementation is treated as single value (driver.Value spec shouldn't return slices)
		{name: "Valuer returns slice not expanded", args: []any{fakeValuer{val: []int{1, 2}}}, want: false},
		// Slice pointer: anti-pattern, let the error surface naturally at driver level
		{name: "slice pointer not recognized", args: []any{&[]int{1, 2}}, want: false},
		// Array differs from slice: In's asSliceForIn only recognizes Slice, not Array
		{name: "array not expanded", args: []any{[3]int{1, 2, 3}}, want: true}, // needsInRewrite uses reflect.Kind which includes Array
		{name: "nil element", args: []any{nil, []int{1, 2}}, want: true},
		{name: "empty slice", args: []any{[]int{}}, want: true},
		{name: "Valuer returns nil", args: []any{fakeValuer{val: nil}}, want: false},
		// AsValue / AsList wrappers: must go through In path (for unwrapping)
		{name: "AsValue wraps scalar must go through In", args: []any{AsValue(42)}, want: true},
		{name: "AsValue wraps slice must go through In", args: []any{AsValue([]int{1, 2})}, want: true},
		{name: "AsList wraps slice must go through In", args: []any{AsList([]int{1, 2})}, want: true},
		{name: "AsValue mixed with plain scalars", args: []any{1, AsValue("foo"), true}, want: true},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := needsInRewrite(c.args)
			if got != c.want {
				t.Errorf("needsInRewrite(%#v) = %v, want %v", c.args, got, c.want)
			}
		})
	}
}

// TestNextPlaceholder unit-tests the internal helper to ensure In's scan logic is correct.
// Also validates the inParen return value — the core of IN list context recognition.
func TestNextPlaceholder(t *testing.T) {
	cases := []struct {
		name        string
		query       string
		start       int
		wantIdx     int
		wantInParen bool
	}{
		// ===== Basic placeholder recognition =====
		{name: "simple ?", query: "SELECT ?", start: 0, wantIdx: 7, wantInParen: false},
		{name: "? inside string skipped", query: "SELECT 'a?' AND ?", start: 0, wantIdx: 16, wantInParen: false},
		{name: "?? skipped", query: "SELECT ?? AND ?", start: 0, wantIdx: 14, wantInParen: false},
		{name: "\\? skipped", query: `SELECT \? AND ?`, start: 0, wantIdx: 14, wantInParen: false},
		{name: "no placeholder", query: "SELECT 1", start: 0, wantIdx: -1, wantInParen: false},
		{name: "line comment skipped", query: "-- ?\nSELECT ?", start: 0, wantIdx: 12, wantInParen: false},
		{name: "block comment skipped", query: "/* ? */ SELECT ?", start: 0, wantIdx: 15, wantInParen: false},
		{name: "PG double-quoted skipped", query: `"col?" SELECT ?`, start: 0, wantIdx: 14, wantInParen: false},
		{name: "MySQL backtick skipped", query: "`col?` SELECT ?", start: 0, wantIdx: 14, wantInParen: false},
		{name: "dollar quoting skipped", query: "$$?$$ SELECT ?", start: 0, wantIdx: 13, wantInParen: false},
		{name: "tagged dollar quoting", query: "$t$?$t$ SELECT ?", start: 0, wantIdx: 15, wantInParen: false},

		// ===== IN list context recognition (strict (?) form + preceded by IN) =====
		{name: "IN_(?)_tight", query: "WHERE id IN (?)", start: 0, wantIdx: 13, wantInParen: true},
		{name: "nested_(?)", query: "(?)", start: 0, wantIdx: 1, wantInParen: false}, // not preceded by IN
		{name: "VALUES_(?)_not_IN", query: "VALUES (?)", start: 0, wantIdx: 8, wantInParen: false},
		{name: "(?_AND_?_second_after_=)", query: "(?) AND x = ?", start: 0, wantIdx: 1, wantInParen: false}, // not preceded by IN
		{name: "IN(_space_?)", query: "WHERE id IN ( ?)", start: 0, wantIdx: 14, wantInParen: true},
		{name: "IN(?_space_)", query: "WHERE id IN (? )", start: 0, wantIdx: 13, wantInParen: true},
		{name: "IN(_space_?_space_)", query: "WHERE id IN ( ? )", start: 0, wantIdx: 14, wantInParen: true},
		{name: "IN(_tab_?_tab_)", query: "WHERE id IN (\t?\t)", start: 0, wantIdx: 14, wantInParen: true},
		{name: "IN(_newline_?_newline_)_multiline", query: "WHERE id IN (\n    ?\n)", start: 0, wantIdx: 18, wantInParen: true},

		// Not matched: ? not in IN (?) context
		{name: "WHERE_=?_no_paren", query: "WHERE x = ?", start: 0, wantIdx: 10, wantInParen: false},
		{name: "(?,?,?)_multi_?_first", query: "(?, ?, ?)", start: 0, wantIdx: 1, wantInParen: false},
		{name: "(?,?,?)_multi_?_second", query: "(?, ?, ?)", start: 2, wantIdx: 4, wantInParen: false},
		{name: "(?+1)_arithmetic", query: "(? + 1)", start: 0, wantIdx: 1, wantInParen: false},
		{name: "(?_IS_NULL)_expression", query: "(? IS NULL)", start: 0, wantIdx: 1, wantInParen: false},
		{name: "(SELECT_?)_subquery", query: "(SELECT ?)", start: 0, wantIdx: 8, wantInParen: false},
		{name: "ANY(?)_preceded_by_ANY_not_IN_inParen=false", query: "= ANY(?)", start: 0, wantIdx: 6, wantInParen: false},
		{name: "(?_comment_)_comment_breaks_match", query: "(? /*c*/)", start: 0, wantIdx: 1, wantInParen: false},
		{name: "(_comment_?)_left_comment_breaks_match", query: "(/*c*/ ?)", start: 0, wantIdx: 7, wantInParen: false},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			gotIdx, gotInParen := nextPlaceholder(c.query, c.start)
			if gotIdx != c.wantIdx || gotInParen != c.wantInParen {
				t.Errorf("nextPlaceholder(%q, %d) = (%d, %v), want (%d, %v)",
					c.query, c.start, gotIdx, gotInParen, c.wantIdx, c.wantInParen)
			}
		})
	}
}

// pqArrayLike simulates pq.Array-style Valuer wrapper:
// .Value() returns a PG array literal string (e.g. "{1,2,3}"), not a slice.
// Used to verify that autoIn does not incorrectly expand when users wrap with such types.
type pqArrayLike struct {
	vals []int
}

func (a pqArrayLike) Value() (driver.Value, error) {
	if a.vals == nil {
		return nil, nil
	}
	s := "{"
	for i, v := range a.vals {
		if i > 0 {
			s += ","
		}
		s += string(rune('0' + v)) // simplified: single-digit integers only, for testing
	}
	return s + "}", nil
}

// TestAutoIn_SliceFieldValue_Boundary validates the "slice as field value" scenario:
// when users wrap slices via driver.Valuer (e.g. pq.Array) as field values,
// autoIn should not expand them into IN lists. This is the autoIn boundary contract.
func TestAutoIn_SliceFieldValue_Boundary(t *testing.T) {
	t.Run("Valuer_wraps_slice_INSERT_not_expanded", func(t *testing.T) {
		// Simulate: INSERT INTO users (tags) VALUES (?)
		// args: pq.Array([]int{1,2,3}) -> should be passed as single value
		query := `INSERT INTO users (tags) VALUES (?)`
		gotQ, gotArgs, err := autoIn(query, pqArrayLike{vals: []int{1, 2, 3}})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		// Key assertion: query not rewritten
		if gotQ != query {
			t.Errorf("query was incorrectly rewritten:\n  got =%q\n  want=%q", gotQ, query)
		}
		// Key assertion: args still 1, and type is Valuer wrapper
		if len(gotArgs) != 1 {
			t.Errorf("args were incorrectly expanded: got len=%d, want 1", len(gotArgs))
		}
	})

	t.Run("byte_slice_INSERT_not_expanded", func(t *testing.T) {
		// MySQL JSON column: serialized as []byte before writing
		query := `INSERT INTO t (json_col) VALUES (?)`
		jsonData := []byte(`[1,2,3]`)
		gotQ, gotArgs, err := autoIn(query, jsonData)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if gotQ != query {
			t.Errorf("query was incorrectly rewritten:\n  got =%q\n  want=%q", gotQ, query)
		}
		if len(gotArgs) != 1 {
			t.Errorf("args were incorrectly expanded: got len=%d, want 1", len(gotArgs))
		}
	})

	t.Run("plain_slice_IN_should_expand", func(t *testing.T) {
		// Control group: unwrapped slice + IN clause -> should expand
		query := `SELECT * FROM users WHERE id IN (?)`
		gotQ, gotArgs, err := autoIn(query, []int{1, 2, 3})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		want := `SELECT * FROM users WHERE id IN (?, ?, ?)`
		if gotQ != want {
			t.Errorf("query mismatch:\n  got =%q\n  want=%q", gotQ, want)
		}
		if len(gotArgs) != 3 {
			t.Errorf("args should expand to 3, got len=%d", len(gotArgs))
		}
	})

	t.Run("plain_slice_INSERT_no_expand_IN_context_recognition", func(t *testing.T) {
		// IN list context recognition: VALUES (?) is preceded by VALUES not IN -> no expansion.
		// Entire slice passed as single value. Use AsList to force expansion.
		query := `INSERT INTO users (tags) VALUES (?)`
		gotQ, gotArgs, err := autoIn(query, []int{1, 2, 3})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if gotQ != query {
			t.Errorf("query should not be rewritten:\n  got =%q\n  want=%q", gotQ, query)
		}
		if len(gotArgs) != 1 {
			t.Errorf("args should remain single value: got len=%d, want 1", len(gotArgs))
		}
	})
}

// TestAutoIn_ParenContext_GORMStyle validates the "paren context recognition" introduced in Phase 1.8.
// Core rule (referring to GORM's afterParenthesis):
//   - ? immediately after ( + slice -> expand (e.g. IN (?))
//   - ? not after ( + slice -> no expand, passed as single value (e.g. WHERE x = ?)
func TestAutoIn_ParenContext_GORMStyle(t *testing.T) {
	t.Run("WHERE_=_slice_no_expand", func(t *testing.T) {
		// User passes slice but ? is not in IN(?) context -> no expand
		query := `SELECT * WHERE x = ?`
		gotQ, gotArgs, err := autoIn(query, []int{1, 2, 3})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if gotQ != query {
			t.Errorf("query should not be rewritten:\n  got =%q\n  want=%q", gotQ, query)
		}
		if len(gotArgs) != 1 {
			t.Errorf("args should remain single value slice: got len=%d, want 1", len(gotArgs))
		}
	})

	t.Run("UPDATE_SET_slice_no_expand", func(t *testing.T) {
		query := `UPDATE t SET tags = ? WHERE id = ?`
		gotQ, gotArgs, err := autoIn(query, []int{1, 2, 3}, 100)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if gotQ != query {
			t.Errorf("query should not be rewritten:\n  got =%q\n  want=%q", gotQ, query)
		}
		if len(gotArgs) != 2 {
			t.Errorf("args should remain 2: got len=%d", len(gotArgs))
		}
	})

	t.Run("IN_(?)_still_expands", func(t *testing.T) {
		query := `SELECT * WHERE id IN (?)`
		gotQ, gotArgs, err := autoIn(query, []int{1, 2, 3})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		want := `SELECT * WHERE id IN (?, ?, ?)`
		if gotQ != want {
			t.Errorf("query mismatch:\n  got =%q\n  want=%q", gotQ, want)
		}
		if len(gotArgs) != 3 {
			t.Errorf("args should expand to 3, got len=%d", len(gotArgs))
		}
	})

	t.Run("INSERT_VALUES_(?)_does_not_expand_IN_context_recognition", func(t *testing.T) {
		// IN list context recognition: VALUES (?) has VALUES before (, not IN -> no expansion.
		// Entire slice passed as single value. Use AsList to force expansion.
		query := `INSERT INTO t (tags) VALUES (?)`
		gotQ, gotArgs, err := autoIn(query, []int{1, 2, 3})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if gotQ != query {
			t.Errorf("query should not be rewritten:\n  got =%q\n  want=%q", gotQ, query)
		}
		if len(gotArgs) != 1 {
			t.Errorf("args should remain single value: got len=%d, want 1", len(gotArgs))
		}
	})

	t.Run("mixed_=_and_IN_(?)", func(t *testing.T) {
		// x = ? with 100; id IN (?) with [1,2]; y = ? with 200
		query := `WHERE x = ? AND id IN (?) AND y = ?`
		gotQ, gotArgs, err := autoIn(query, 100, []int{1, 2}, 200)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		want := `WHERE x = ? AND id IN (?, ?) AND y = ?`
		if gotQ != want {
			t.Errorf("query mismatch:\n  got =%q\n  want=%q", gotQ, want)
		}
		if len(gotArgs) != 4 {
			t.Errorf("args should be 4: got len=%d", len(gotArgs))
		}
	})
}

// TestAutoIn_AsValueAsList_EscapeHooks validates sqlex.AsValue / sqlex.AsList escape helpers.
func TestAutoIn_AsValueAsList_EscapeHooks(t *testing.T) {
	t.Run("AsValue_wraps_slice_INSERT_no_expand", func(t *testing.T) {
		// AsValue prevents expansion even when ? is in IN (?) context
		query := `INSERT INTO t (tags) VALUES (?)`
		gotQ, gotArgs, err := autoIn(query, AsValue([]int{1, 2, 3}))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if gotQ != query {
			t.Errorf("query should not be rewritten:\n  got =%q\n  want=%q", gotQ, query)
		}
		if len(gotArgs) != 1 {
			t.Errorf("args should remain single value: got len=%d, want 1", len(gotArgs))
		}
		// Verify args[0] is the original slice (not unwrapped)
		if got, ok := gotArgs[0].([]int); !ok || len(got) != 3 {
			t.Errorf("args[0] should be []int{1,2,3}: got %v (%T)", gotArgs[0], gotArgs[0])
		}
	})

	t.Run("AsValue_wraps_non_slice_scalar", func(t *testing.T) {
		query := `WHERE x = ?`
		gotQ, gotArgs, err := autoIn(query, AsValue(42))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if gotQ != query {
			t.Errorf("query should not be rewritten")
		}
		if gotArgs[0] != 42 {
			t.Errorf("args[0] should be 42: got %v", gotArgs[0])
		}
	})

	t.Run("AsList_slice_WHERE=_force_expand", func(t *testing.T) {
		// Business explicitly wants expansion (even though ? is not in IN (?) context)
		query := `WHERE x = ?`
		gotQ, gotArgs, err := autoIn(query, AsList([]int{1, 2, 3}))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		want := `WHERE x = ?, ?, ?`
		if gotQ != want {
			t.Errorf("query should be expanded:\n  got =%q\n  want=%q", gotQ, want)
		}
		if len(gotArgs) != 3 {
			t.Errorf("args should expand to 3: got len=%d", len(gotArgs))
		}
	})

	t.Run("AsList_non_slice_should_error", func(t *testing.T) {
		_, _, err := autoIn(`WHERE x = ?`, AsList(42))
		if err == nil {
			t.Fatal("expected error for non-slice AsList, got nil")
		}
		if !strings.Contains(err.Error(), "AsList: argument is not a slice") {
			t.Errorf("error should contain 'AsList: argument is not a slice': got %v", err)
		}
	})

	t.Run("AsList_array_should_error", func(t *testing.T) {
		// Array (non-slice): asSliceForIn only recognizes reflect.Slice; AsList should reject consistently
		_, _, err := autoIn(`WHERE x IN (?)`, AsList([3]int{1, 2, 3}))
		if err == nil {
			t.Fatal("expected error for array AsList, got nil")
		}
		if !strings.Contains(err.Error(), "AsList: argument is not a slice") {
			t.Errorf("error should contain 'AsList: argument is not a slice': got %v", err)
		}
	})

	t.Run("AsList_byte_slice_should_error", func(t *testing.T) {
		// []byte is a standard driver.Value type, explicitly excluded by asSliceForIn;
		// AsList is consistent — reject []byte forced expansion (avoid misidentifying single value as list)
		_, _, err := autoIn(`WHERE x IN (?)`, AsList([]byte{1, 2, 3}))
		if err == nil {
			t.Fatal("expected error for []byte AsList, got nil")
		}
		if !strings.Contains(err.Error(), "AsList: argument is not a slice") {
			t.Errorf("error should contain 'AsList: argument is not a slice': got %v", err)
		}
	})

	t.Run("AsList_empty_slice_should_error", func(t *testing.T) {
		_, _, err := autoIn(`WHERE x = ?`, AsList([]int{}))
		if err == nil {
			t.Fatal("expected error for empty slice AsList")
		}
		if !strings.Contains(err.Error(), "AsList: empty slice") {
			t.Errorf("error should contain 'AsList: empty slice': got %v", err)
		}
	})

	t.Run("AsValue_and_AsList_mixed", func(t *testing.T) {
		// VALUES (?, ?, ?): none are in IN (?) context -> all default no expand
		// Use AsValue for 1st, AsList to force expand 2nd/3rd
		query := `INSERT INTO t (tags, ids, others) VALUES (?, ?, ?)`
		gotQ, gotArgs, err := autoIn(query,
			AsValue([]int{1, 2}),   // 1st ? not IN(?) + AsValue -> no expand
			AsList([]int{3, 4, 5}), // 2nd ? not IN(?) + AsList -> force expand
			AsList([]int{6, 7}),    // 3rd ? not IN(?) + AsList -> force expand
		)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		// ?1 = AsValue no expand (keep ?)
		// ?2 = AsList force expand to ?, ?, ?
		// ?3 = AsList force expand to ?, ?
		want := `INSERT INTO t (tags, ids, others) VALUES (?, ?, ?, ?, ?, ?)`
		if gotQ != want {
			t.Errorf("query mismatch:\n  got =%q\n  want=%q", gotQ, want)
		}
		// args = [AsValue slice, 3, 4, 5, 6, 7] = 6 total
		if len(gotArgs) != 6 {
			t.Errorf("args should be 6: got len=%d, %v", len(gotArgs), gotArgs)
		}
	})

	t.Run("AsValue_wraps_pq.Array_style_Valuer", func(t *testing.T) {
		// AsValue takes priority over Valuer: AsValue doesn't unwrap, original value passed to driver
		query := `INSERT INTO t (tags) VALUES (?)`
		v := pqArrayLike{vals: []int{1, 2}}
		gotQ, gotArgs, err := autoIn(query, AsValue(v))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if gotQ != query {
			t.Errorf("query should not be rewritten")
		}
		if len(gotArgs) != 1 {
			t.Errorf("args should remain single value: got len=%d", len(gotArgs))
		}
		// args[0] should be the original pqArrayLike, not Value()-unwrapped — driver handles it
		if _, ok := gotArgs[0].(pqArrayLike); !ok {
			t.Errorf("args[0] should be pqArrayLike: got %T", gotArgs[0])
		}
	})

	t.Run("AsValue_wraps_empty_slice_valid_as_single_value", func(t *testing.T) {
		// AsValue doesn't require non-empty (unlike AsList), entire slice as single value to driver
		query := `INSERT INTO t (tags) VALUES (?)`
		gotQ, gotArgs, err := autoIn(query, AsValue([]int{}))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if gotQ != query {
			t.Errorf("query should not be rewritten:\n  got =%q\n  want=%q", gotQ, query)
		}
		if len(gotArgs) != 1 {
			t.Errorf("args should remain single value: got len=%d, want 1", len(gotArgs))
		}
		if got, ok := gotArgs[0].([]int); !ok || len(got) != 0 {
			t.Errorf("args[0] should be []int{}: got %v (%T)", gotArgs[0], gotArgs[0])
		}
	})

	t.Run("AsList_in_IN(?)_context_still_expands_same_as_bare_slice", func(t *testing.T) {
		// AsList is "force expand", equivalent to bare slice in IN (?) context
		query := `WHERE id IN (?)`
		gotQ, gotArgs, err := autoIn(query, AsList([]int{1, 2, 3}))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		want := `WHERE id IN (?, ?, ?)`
		if gotQ != want {
			t.Errorf("query should expand:\n  got =%q\n  want=%q", gotQ, want)
		}
		if len(gotArgs) != 3 {
			t.Errorf("args should expand to 3: got len=%d", len(gotArgs))
		}
	})
}

// TestAutoIn_StrictParen_E2E end-to-end validates IN list context recognition —
// focusing on multi-line/Tab/whitespace SQL forms that "look likely to misidentify",
// verifying nextPlaceholder and In cooperate to correctly identify IN (?) and expand.
func TestAutoIn_StrictParen_E2E(t *testing.T) {
	t.Run("multiline_IN(\\n_?\\n)_expands", func(t *testing.T) {
		query := "WHERE id IN (\n    ?\n)"
		gotQ, gotArgs, err := autoIn(query, []int{1, 2, 3})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		// Multi-line SQL should be recognized and expanded (preserving original whitespace)
		want := "WHERE id IN (\n    ?, ?, ?\n)"
		if gotQ != want {
			t.Errorf("query mismatch:\n  got =%q\n  want=%q", gotQ, want)
		}
		if len(gotArgs) != 3 {
			t.Errorf("args should expand to 3: got len=%d", len(gotArgs))
		}
	})

	t.Run("(_space_?_space_)_expands", func(t *testing.T) {
		query := `WHERE id IN ( ? )`
		gotQ, gotArgs, err := autoIn(query, []int{10, 20})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		// Preserves original whitespace: left space after ( and right space before )
		want := `WHERE id IN ( ?, ? )`
		if gotQ != want {
			t.Errorf("query mismatch:\n  got =%q\n  want=%q", gotQ, want)
		}
		if len(gotArgs) != 2 {
			t.Errorf("args should expand to 2: got len=%d", len(gotArgs))
		}
	})

	t.Run("(?,?,?)_multi_?_no_expand_each_?_maps_to_scalar", func(t *testing.T) {
		// Multiple ? treated as user already expanded, each ? maps to a scalar, no In expansion
		query := `WHERE id IN (?, ?, ?)`
		gotQ, gotArgs, err := autoIn(query, 1, 2, 3)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if gotQ != query {
			t.Errorf("query should not be rewritten:\n  got =%q\n  want=%q", gotQ, query)
		}
		if len(gotArgs) != 3 {
			t.Errorf("args should remain 3 scalars: got len=%d", len(gotArgs))
		}
	})

	t.Run("ANY(?)_slice_IN_context_recognition_default_no_expand", func(t *testing.T) {
		// IN list context recognition: ANY(?) is preceded by ANY not IN -> default no expand,
		// entire slice passed as single value to driver (correct behavior for PG array params),
		// no AsValue needed.
		query1 := `WHERE id = ANY(?)`
		gotQ1, gotArgs1, err := autoIn(query1, []int{1, 2, 3})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if gotQ1 != query1 {
			t.Errorf("ANY(?) should not be expanded: got %q", gotQ1)
		}
		if len(gotArgs1) != 1 {
			t.Errorf("args should remain single value slice: got len=%d, want 1", len(gotArgs1))
		}

		// If business needs expansion, use AsList to force
		gotQ2, gotArgs2, err := autoIn(query1, AsList([]int{1, 2, 3}))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if gotQ2 != `WHERE id = ANY(?, ?, ?)` {
			t.Errorf("AsList should force expansion: got %q", gotQ2)
		}
		if len(gotArgs2) != 3 {
			t.Errorf("AsList should expand to 3 args: got len=%d", len(gotArgs2))
		}
	})
}

// TestAutoIn_EmptySlice_ContextSensitive validates context-sensitive empty slice handling:
//   - IN (?) context + empty slice -> error (IN () is invalid SQL)
//   - Non-IN context + empty slice -> no error, entire slice passed to driver
//   - AsValue wrapping empty slice -> no error (forced single value, covered elsewhere)
//   - AsList wrapping empty slice -> error (expanding to nothing is meaningless)
func TestAutoIn_EmptySlice_ContextSensitive(t *testing.T) {
	t.Run("IN(?)_context_empty_slice_errors", func(t *testing.T) {
		query := `SELECT * FROM t WHERE id IN (?)`
		_, _, err := autoIn(query, []int{})
		if err == nil {
			t.Fatal("IN (?) context + empty slice should error, but got nil")
		}
		if !strings.Contains(err.Error(), "empty slice") {
			t.Errorf("error should contain 'empty slice': got %v", err)
		}
		if !strings.Contains(err.Error(), "IN ()") {
			t.Errorf("error should explain IN () is invalid SQL: got %v", err)
		}
	})

	t.Run("non_IN_context_WHERE=?_empty_slice_no_error", func(t *testing.T) {
		query := `SELECT * FROM t WHERE x = ?`
		gotQ, gotArgs, err := autoIn(query, []int{})
		if err != nil {
			t.Fatalf("non-IN context + empty slice should not error: got %v", err)
		}
		if gotQ != query {
			t.Errorf("query should not be rewritten: got %q", gotQ)
		}
		if len(gotArgs) != 1 {
			t.Errorf("args should remain 1 (entire slice): got len=%d", len(gotArgs))
		}
		if got, ok := gotArgs[0].([]int); !ok || len(got) != 0 {
			t.Errorf("args[0] should be empty slice []int{}: got %v (%T)", gotArgs[0], gotArgs[0])
		}
	})

	t.Run("non_IN_context_UPDATE_SET_empty_slice_no_error", func(t *testing.T) {
		query := `UPDATE users SET tags = ? WHERE id = ?`
		gotQ, gotArgs, err := autoIn(query, []int{}, 100)
		if err != nil {
			t.Fatalf("non-IN context + empty slice should not error: got %v", err)
		}
		if gotQ != query {
			t.Errorf("query should not be rewritten: got %q", gotQ)
		}
		if len(gotArgs) != 2 {
			t.Errorf("args should remain 2: got len=%d", len(gotArgs))
		}
	})

	t.Run("non_IN_context_VALUES_multi_?_empty_slice_no_error", func(t *testing.T) {
		// VALUES (?, ?, ?): 2nd ? is not in IN(?) context (preceded by ,), empty slice should not error
		query := `INSERT INTO t (a, b, c) VALUES (?, ?, ?)`
		gotQ, gotArgs, err := autoIn(query, "x", []int{}, "z")
		if err != nil {
			t.Fatalf("non-IN context + empty slice should not error: got %v", err)
		}
		if gotQ != query {
			t.Errorf("query should not be rewritten: got %q", gotQ)
		}
		if len(gotArgs) != 3 {
			t.Errorf("args should remain 3: got len=%d", len(gotArgs))
		}
		if got, ok := gotArgs[1].([]int); !ok || len(got) != 0 {
			t.Errorf("args[1] should be empty slice: got %v (%T)", gotArgs[1], gotArgs[1])
		}
	})

	t.Run("mixed_IN(?)_empty_slice_errors_others_unaffected", func(t *testing.T) {
		// Multiple ?: if any is in IN(?) context + empty slice, the whole call should error
		query := `WHERE x = ? AND id IN (?) AND y = ?`
		_, _, err := autoIn(query, "foo", []int{}, "bar")
		if err == nil {
			t.Fatal("IN (?) + empty slice should error")
		}
		if !strings.Contains(err.Error(), "empty slice") {
			t.Errorf("error should contain 'empty slice': got %v", err)
		}
	})

	t.Run("AsValue_empty_slice_valid", func(t *testing.T) {
		// Confirm: even in IN(?) context, AsValue prevents expansion -> no error
		query := `INSERT INTO t (tags) VALUES (?)`
		_, gotArgs, err := autoIn(query, AsValue([]int{}))
		if err != nil {
			t.Fatalf("AsValue + empty slice should not error: got %v", err)
		}
		if len(gotArgs) != 1 {
			t.Errorf("args should remain 1: got len=%d", len(gotArgs))
		}
	})
}

// TestIn_ArgCountMismatch validates In's argument count mismatch error handling.
//
// Prerequisite: at least one slice / AsValue / AsList must be present to trigger needRewrite
// and enter the main loop; pure scalar cases take the fast path and return directly (driver handles count).
//
// This is the hard guarantee of In's interface contract — users passing too few or too many args
// must get a clear error, not a delayed driver-level error.
func TestIn_ArgCountMismatch(t *testing.T) {
	t.Run("?_exceeds_args_reports_exceeds_arguments", func(t *testing.T) {
		// 2 ?, only 1 arg (and it's a slice, triggering needRewrite)
		_, _, err := In(`WHERE a IN (?) AND b = ?`, []int{1, 2})
		if err == nil {
			t.Fatal("expected error for ? exceeds args, got nil")
		}
		if !strings.Contains(err.Error(), "number of bindVars exceeds arguments") {
			t.Errorf("error should contain 'number of bindVars exceeds arguments': got %v", err)
		}
	})

	t.Run("args_exceed_?_reports_less_than", func(t *testing.T) {
		// 1 ?, 2 args (at least one slice)
		_, _, err := In(`WHERE a IN (?)`, []int{1, 2}, []int{3, 4})
		if err == nil {
			t.Fatal("expected error for args exceeds ?, got nil")
		}
		if !strings.Contains(err.Error(), "number of bindVars less than number arguments") {
			t.Errorf("error should contain 'number of bindVars less than number arguments': got %v", err)
		}
	})

	t.Run("AsValue_also_triggers_count_check", func(t *testing.T) {
		// AsValue triggers needRewrite=true, entering main loop even without slice expansion
		_, _, err := In(`WHERE a = ? AND b = ?`, AsValue(1))
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if !strings.Contains(err.Error(), "exceeds arguments") {
			t.Errorf("error should contain 'exceeds arguments': got %v", err)
		}
	})

	t.Run("AsList_also_triggers_count_check", func(t *testing.T) {
		_, _, err := In(`WHERE a = ?`, AsList([]int{1, 2}), AsList([]int{3, 4}))
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if !strings.Contains(err.Error(), "less than number arguments") {
			t.Errorf("error should contain 'less than number arguments': got %v", err)
		}
	})

	t.Run("pure_scalar_count_mismatch_fast_path_no_error", func(t *testing.T) {
		// Reverse regression: pure scalars don't enter main loop, In does no count check
		// This is the current design trade-off — errors surface at driver level
		gotQ, gotArgs, err := In(`WHERE a = ? AND b = ?`, 1)
		if err != nil {
			t.Fatalf("pure scalar fast path should not error: got %v", err)
		}
		if gotQ != `WHERE a = ? AND b = ?` || len(gotArgs) != 1 {
			t.Errorf("fast path should pass through as-is: got query=%q args=%v", gotQ, gotArgs)
		}
	})
}
