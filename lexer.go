package sqlex

// lexer.go — unified SQL lexical scanner.
//
// compileNamedQuery / Rebind / In(nextPlaceholder) all need to skip string literals,
// quoted identifiers, dollar-quoted strings, and comments to avoid mistaking ? / :name
// within them for placeholders. This file extracts a unified implementation shared by
// all three, preventing "fix one, break two" drift.
//
// Design principles:
//   - No full SQL parsing (YAGNI); only identifies "lexical regions that may contain bare ? / :name".
//   - Byte-level scanning: all lexical delimiters are ASCII, and UTF-8 multi-byte continuation
//     bytes are >= 0x80 so they never conflict, making byte scanning safe.
//
// Known edge cases (rare scenarios):
//   - Block comments do not nest (ends at the first */; PG supports nesting but SQL standard does not require it)
//   - Single quotes only recognize SQL-standard '' escaping, not MySQL backslash escaping \'
//   - Does not recognize E'...' / U&'...' and other prefixed string variants

// sqlSegmentKind identifies the lexical type of a SQL text segment.
type sqlSegmentKind int

const (
	// segNormal normal SQL text — placeholders (? / :name) are valid in this region.
	segNormal sqlSegmentKind = iota
	// segSingleQuote single-quoted string literal '...' (with '' escaping).
	segSingleQuote
	// segDoubleQuote double-quoted identifier "..." (PostgreSQL, with "" escaping).
	segDoubleQuote
	// segBacktick backtick-quoted identifier `...` (MySQL, with `` escaping).
	segBacktick
	// segBracket bracket-quoted identifier [...] (SQL Server, with ]] escaping).
	segBracket
	// segDollarQuote dollar-quoted string $tag$...$tag$ (PostgreSQL).
	segDollarQuote
	// segLineComment line comment -- ...\n.
	segLineComment
	// segBlockComment block comment /* ... */.
	segBlockComment
)

// scanSkipSegment determines whether query[start] is the start of a "lexical segment to skip".
//
// On match: returns (end, kind, true), where query[start:end] is the complete lexical segment
// (including delimiters). The caller should preserve this segment verbatim and continue scanning from end.
// On no match: returns (start, segNormal, false); the caller should process query[start] as a normal character.
func scanSkipSegment(query string, start int) (end int, kind sqlSegmentKind, skip bool) {
	if start >= len(query) {
		return start, segNormal, false
	}

	switch query[start] {
	case '\'':
		return scanQuoted(query, start, '\''), segSingleQuote, true
	case '"':
		return scanQuoted(query, start, '"'), segDoubleQuote, true
	case '`':
		return scanQuoted(query, start, '`'), segBacktick, true
	case '[':
		return scanBracketIdentifier(query, start), segBracket, true
	case '$':
		if end, ok := scanDollarQuote(query, start); ok {
			return end, segDollarQuote, true
		}
		// Not dollar quoting; treat as a normal character
		return start, segNormal, false
	case '-':
		if start+1 < len(query) && query[start+1] == '-' {
			return scanLineComment(query, start), segLineComment, true
		}
		return start, segNormal, false
	case '/':
		if start+1 < len(query) && query[start+1] == '*' {
			return scanBlockComment(query, start), segBlockComment, true
		}
		return start, segNormal, false
	}
	return start, segNormal, false
}

// scanBracketIdentifier scans a SQL Server bracket-quoted identifier [...].
// Supports ]] escaping (in SQL Server, ]] inside brackets represents a literal ]).
// start points to '['. Returns the position after the closing ']'.
// If unclosed, returns len(query).
func scanBracketIdentifier(query string, start int) int {
	i := start + 1
	for i < len(query) {
		if query[i] == ']' {
			// ]] is an escape, representing a literal ]; stay in the region
			if i+1 < len(query) && query[i+1] == ']' {
				i += 2
				continue
			}
			return i + 1 // After the closing ]
		}
		i++
	}
	return len(query) // Unclosed
}

// scanQuoted scans a region delimited by the quote character (single/double/backtick quotes),
// supporting the SQL-standard escape of two consecutive quotes representing a literal quote (” / "" / “).
// start points to the opening quote. Returns the position after the closing quote (exclusive).
// If unclosed, returns len(query) (delegated to the driver for error reporting; this scanner
// does not perform SQL validation).
func scanQuoted(query string, start int, quote byte) int {
	i := start + 1
	for i < len(query) {
		if query[i] == quote {
			// Two consecutive quotes is an escape; stay in the region
			if i+1 < len(query) && query[i+1] == quote {
				i += 2
				continue
			}
			return i + 1 // After the closing quote
		}
		i++
	}
	return len(query) // Unclosed
}

// scanDollarQuote attempts to parse query[start] (expected '$') as the start of a dollar-quoted string.
// On success, returns (position after closing tag, true); if not valid dollar quoting, returns (start, false).
//
// Tag rules: between the two $ characters, only letters, digits, and underscores are allowed
// (the tag can be empty, i.e. $$).
func scanDollarQuote(query string, start int) (end int, ok bool) {
	// Parse open tag: $<tag>$
	tagEnd := start + 1
	for tagEnd < len(query) {
		c := query[tagEnd]
		if c == '$' {
			break
		}
		if !isDollarTagByte(c) {
			return start, false
		}
		tagEnd++
	}
	if tagEnd >= len(query) || query[tagEnd] != '$' {
		return start, false
	}

	// Open tag is query[start : tagEnd+1] (including both $), closing string is the same
	closing := query[start : tagEnd+1]
	i := tagEnd + 1
	for i < len(query) {
		if i+len(closing) <= len(query) && query[i:i+len(closing)] == closing {
			return i + len(closing), true
		}
		i++
	}
	return len(query), true // Unclosed; entire segment treated as dollar quote
}

// isDollarTagByte determines whether a character can be used in a dollar quoting tag (letters/digits/underscore).
func isDollarTagByte(c byte) bool {
	return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') ||
		(c >= '0' && c <= '9') || c == '_'
}

// scanLineComment scans a line comment -- ..., returning the position after the newline
// (including the newline), or to the end of the string. start points to the first '-'.
func scanLineComment(query string, start int) int {
	i := start + 2 // Skip --
	for i < len(query) {
		if query[i] == '\n' {
			return i + 1
		}
		i++
	}
	return len(query)
}

// scanBlockComment scans a block comment /* ... */, returning the position after the closing */.
// start points to '/'. Nesting is not supported (ends at the first */). Returns len(query) if unclosed.
func scanBlockComment(query string, start int) int {
	i := start + 2 // Skip /*
	for i < len(query) {
		if query[i] == '*' && i+1 < len(query) && query[i+1] == '/' {
			return i + 2
		}
		i++
	}
	return len(query)
}
