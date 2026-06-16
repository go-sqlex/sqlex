package sqlex

import "testing"

// TestRebind_EdgeCases covers Rebind behavior across various SQL lexical elements.
// Goal: maintain symmetry with compileNamedQuery's lexical skip rules — all ? within
// SQL lexical scopes (string literals, double-quoted identifiers, backtick identifiers,
// dollar-quoted strings, comments) will not be replaced as placeholders.
func TestRebind_EdgeCases(t *testing.T) {
	cases := []struct {
		name     string
		query    string
		bindType int
		want     string
	}{
		// ===== Single-quoted string literal (existing feature, retained as baseline) =====
		{
			name: "question mark inside single-quoted string not replaced", query: `SELECT 'a?b' WHERE x = ?`, bindType: DOLLAR,
			want: `SELECT 'a?b' WHERE x = $1`,
		},

		// ===== Line comment (current fix point) =====
		{
			name:     "question mark inside line comment not replaced",
			query:    "SELECT * FROM t -- WHERE id = ?\nWHERE x = ?",
			bindType: DOLLAR,
			want:     "SELECT * FROM t -- WHERE id = ?\nWHERE x = $1",
		},
		{
			name:     "line comment until end of file",
			query:    "SELECT ? -- comment ?",
			bindType: DOLLAR,
			want:     "SELECT $1 -- comment ?",
		},
		{
			name:     "multiple consecutive line comments",
			query:    "SELECT * -- c1 ?\n-- c2 ?\nFROM t WHERE x = ?",
			bindType: DOLLAR,
			want:     "SELECT * -- c1 ?\n-- c2 ?\nFROM t WHERE x = $1",
		},

		// ===== Block comment (current fix point) =====
		{
			name:     "question mark inside block comment not replaced",
			query:    `SELECT /* WHERE id = ? */ * FROM t WHERE x = ?`,
			bindType: DOLLAR,
			want:     `SELECT /* WHERE id = ? */ * FROM t WHERE x = $1`,
		},
		{
			name:     "block comment spanning multiple lines",
			query:    "SELECT * /* line1\n? line2\n? line3 */ FROM t WHERE x = ?",
			bindType: DOLLAR,
			want:     "SELECT * /* line1\n? line2\n? line3 */ FROM t WHERE x = $1",
		},
		{
			name:     "nested block comment_only outer recognized",
			query:    `SELECT /* a /* b ? */ ? */ * FROM t WHERE x = ?`,
			bindType: DOLLAR,
			// Current implementation does not support SQL nested block comments (a PG-specific extension);
			// the first */ ends the comment, and the ? in the remaining ? */ is replaced.
			// This is a reasonable simplification — mainstream drivers rarely support nested block comments.
			want: `SELECT /* a /* b ? */ $1 */ * FROM t WHERE x = $2`,
		},

		// ===== PG double-quoted identifier (current fix point) =====
		{
			name:     "question mark inside PG double-quoted identifier not replaced",
			query:    `SELECT "col?name" FROM t WHERE x = ?`,
			bindType: DOLLAR,
			want:     `SELECT "col?name" FROM t WHERE x = $1`,
		},
		{
			name:     "PG double-quoted escape",
			query:    `SELECT "a""b?" FROM t WHERE x = ?`,
			bindType: DOLLAR,
			want:     `SELECT "a""b?" FROM t WHERE x = $1`,
		},

		// ===== MySQL backtick identifier (current fix point) =====
		{
			name:     "question mark inside MySQL backtick identifier not replaced",
			query:    "SELECT `col?name` FROM t WHERE x = ?",
			bindType: DOLLAR,
			want:     "SELECT `col?name` FROM t WHERE x = $1",
		},
		{
			name:     "MySQL backtick escape",
			query:    "SELECT `a``b?` FROM t WHERE x = ?",
			bindType: DOLLAR,
			want:     "SELECT `a``b?` FROM t WHERE x = $1",
		},

		// ===== PostgreSQL Dollar Quoting (current fix point) =====
		{
			name:     "question mark inside Dollar_quote not replaced",
			query:    `SELECT $$hello?$$ FROM t WHERE x = ?`,
			bindType: DOLLAR,
			want:     `SELECT $$hello?$$ FROM t WHERE x = $1`,
		},
		{
			name:     "question mark inside Tagged_dollar_quote not replaced",
			query:    `SELECT $tag$?$tag$ FROM t WHERE x = ?`,
			bindType: DOLLAR,
			want:     `SELECT $tag$?$tag$ FROM t WHERE x = $1`,
		},
		{
			name:     "Dollar_quote spanning lines",
			query:    "SELECT $$line1\n? line2\n?$$ FROM t WHERE x = ?",
			bindType: DOLLAR,
			want:     "SELECT $$line1\n? line2\n?$$ FROM t WHERE x = $1",
		},
		{
			name:     "standalone_$_not_dollar_quoting",
			query:    `SELECT 100$ FROM t WHERE x = ?`,
			bindType: DOLLAR,
			want:     `SELECT 100$ FROM t WHERE x = $1`,
		},

		// ===== Escape =====
		{
			name:     "backslash-escaped question mark",
			query:    `SELECT * WHERE x = \?`,
			bindType: DOLLAR,
			want:     `SELECT * WHERE x = ?`,
		},
		{
			name:     "double question mark escape",
			query:    `SELECT * WHERE x = ?? AND y = ?`,
			bindType: DOLLAR,
			want:     `SELECT * WHERE x = ? AND y = $1`,
		},

		// ===== Placeholder-related =====
		{
			name:     "DOLLAR type_multiple placeholders",
			query:    `SELECT * WHERE a = ? AND b = ? AND c = ?`,
			bindType: DOLLAR,
			want:     `SELECT * WHERE a = $1 AND b = $2 AND c = $3`,
		},
		{
			name:     "AT type_SQL_Server",
			query:    `SELECT * WHERE a = ? AND b = ?`,
			bindType: AT,
			want:     `SELECT * WHERE a = @p1 AND b = @p2`,
		},
		{
			name:     "NAMED type_Oracle",
			query:    `SELECT * WHERE a = ? AND b = ?`,
			bindType: NAMED,
			want:     `SELECT * WHERE a = :arg1 AND b = :arg2`,
		},
		{
			name:     "QUESTION type_not replaced",
			query:    `SELECT * WHERE a = ?`,
			bindType: QUESTION,
			want:     `SELECT * WHERE a = ?`,
		},
		{
			name:     "UNKNOWN type_not replaced",
			query:    `SELECT * WHERE a = ?`,
			bindType: UNKNOWN,
			want:     `SELECT * WHERE a = ?`,
		},

		// ===== Edge cases =====
		{
			name:     "empty query",
			query:    ``,
			bindType: DOLLAR,
			want:     ``,
		},
		{
			name:     "whitespace-only query",
			query:    `   `,
			bindType: DOLLAR,
			want:     `   `,
		},
		{
			name:     "question mark at start",
			query:    `? = ?`,
			bindType: DOLLAR,
			want:     `$1 = $2`,
		},
		{
			name:     "question mark at end",
			query:    `SELECT * WHERE x = ?`,
			bindType: DOLLAR,
			want:     `SELECT * WHERE x = $1`,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := Rebind(c.bindType, c.query)
			if got != c.want {
				t.Errorf("Rebind(%d, %q) =\n  got =%q\n  want=%q",
					c.bindType, c.query, got, c.want)
			}
		})
	}
}

// TestRebind_Idempotent_Extended extends the original TestRebindIdempotent with new lexical elements.
func TestRebind_Idempotent_Extended(t *testing.T) {
	queries := []string{
		`SELECT 'a?b' WHERE x = ?`,
		"SELECT * -- ?\nWHERE x = ?",
		`SELECT /* ? */ x FROM t WHERE y = ?`,
		`SELECT "col?" FROM t WHERE x = ?`,
		"SELECT `col?` FROM t WHERE x = ?",
		`SELECT $$?$$ FROM t WHERE x = ?`,
		`SELECT $tag$?$tag$ FROM t WHERE x = ?`,
	}

	bindTypes := []int{DOLLAR, NAMED, AT}
	for _, bt := range bindTypes {
		for _, q := range queries {
			once := Rebind(bt, q)
			twice := Rebind(bt, once)
			// Placeholders produced after the first pass (e.g., $1) are still not ?, so twice should equal once
			// i.e., Rebind is idempotent
			if once != twice {
				t.Errorf("Rebind not idempotent for bindType=%d:\n  query=%q\n  once =%q\n  twice=%q",
					bt, q, once, twice)
			}
		}
	}
}
