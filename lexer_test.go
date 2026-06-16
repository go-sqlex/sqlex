package sqlex

import "testing"

// TestScanSkipSegment verifies the boundary recognition of the unified lexical scanner for various SQL lexical segments.
// This is the shared lexical foundation for Rebind / nextPlaceholder / compileNamedQuery,
// and must be precise — any deviation would cause placeholder recognition errors in all three places simultaneously.
func TestScanSkipSegment(t *testing.T) {
	cases := []struct {
		name     string
		query    string
		start    int
		wantEnd  int
		wantKind sqlSegmentKind
		wantSkip bool
	}{
		// ===== Normal characters (not skipped) =====
		{"normal character_question mark", "?", 0, 0, segNormal, false},
		{"normal character_colon", ":id", 0, 0, segNormal, false},
		{"normal character_letter", "SELECT", 0, 0, segNormal, false},
		{"single minus not comment", "a-b", 1, 1, segNormal, false},
		{"single slash not comment", "a/b", 1, 1, segNormal, false},
		{"dollar not dollar quote", "$1", 0, 0, segNormal, false},

		// ===== Single-quoted string =====
		{"single quote simple", "'abc'", 0, 5, segSingleQuote, true},
		{"single quote with question mark", "'a?b'", 0, 5, segSingleQuote, true},
		{"single quote SQL escape", "'O''Reilly'", 0, 11, segSingleQuote, true},
		{"single quote unclosed", "'abc", 0, 4, segSingleQuote, true},
		{"single quote empty", "''", 0, 2, segSingleQuote, true},

		// ===== Double-quoted identifier =====
		{"double quote simple", `"col"`, 0, 5, segDoubleQuote, true},
		{"double quote with colon", `"a:b"`, 0, 5, segDoubleQuote, true},
		{"double quote escape", `"a""b"`, 0, 6, segDoubleQuote, true},

		// ===== Backtick identifier =====
		{"backtick simple", "`col`", 0, 5, segBacktick, true},
		{"backtick with question mark", "`a?b`", 0, 5, segBacktick, true},
		{"backtick escape", "`a``b`", 0, 6, segBacktick, true},

		// ===== dollar-quoted =====
		{"dollar empty tag", "$$abc$$", 0, 7, segDollarQuote, true},
		{"dollar with tag", "$tag$abc$tag$", 0, 13, segDollarQuote, true},
		{"dollar with question mark", "$$a?b$$", 0, 7, segDollarQuote, true},
		{"dollar with colon", "$$a:b$$", 0, 7, segDollarQuote, true},
		{"dollar unclosed", "$$abc", 0, 5, segDollarQuote, true},
		{"dollar invalid tag fallback", "$1abc", 0, 0, segNormal, false},

		// ===== Line comment =====
		{"line comment with newline", "-- c\nx", 0, 5, segLineComment, true},
		{"line comment to end", "-- comment", 0, 10, segLineComment, true},
		{"line comment with question mark", "-- ?\n", 0, 5, segLineComment, true},

		// ===== Block comment =====
		{"block comment simple", "/* c */x", 0, 7, segBlockComment, true},
		{"block comment with question mark", "/* ? */", 0, 7, segBlockComment, true},
		{"block comment unclosed", "/* abc", 0, 6, segBlockComment, true},
		{"block comment not nested_first star_slash ends", "/* /* x */ y */", 0, 10, segBlockComment, true},

		// ===== Bracket-quoted identifier (SQL Server) =====
		{"bracket simple", "[col]", 0, 5, segBracket, true},
		{"bracket with question mark", "[a?b]", 0, 5, segBracket, true},
		{"bracket with colon", "[a:b]", 0, 5, segBracket, true},
		{"bracket escape ]]", "[a]]b]", 0, 6, segBracket, true},
		{"bracket unclosed", "[abc", 0, 4, segBracket, true},
		{"bracket empty", "[]", 0, 2, segBracket, true},
		{"bracket with space", "[Order Status]", 0, 14, segBracket, true},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			end, kind, skip := scanSkipSegment(c.query, c.start)
			if end != c.wantEnd || kind != c.wantKind || skip != c.wantSkip {
				t.Errorf("scanSkipSegment(%q, %d) = (%d, %d, %v), want (%d, %d, %v)",
					c.query, c.start, end, kind, skip, c.wantEnd, c.wantKind, c.wantSkip)
			}
		})
	}
}
