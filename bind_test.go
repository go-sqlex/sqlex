package sqlex

import (
	"math/rand"
	"testing"
)

func oldBindType(driverName string) int {
	switch driverName {
	case "postgres", "pgx", "pq-timeouts", "cloudsqlpostgres", "ql":
		return DOLLAR
	case "mysql":
		return QUESTION
	case "sqlite3":
		return QUESTION
	case "oci8", "ora", "goracle", "godror":
		return NAMED
	case "sqlserver":
		return AT
	}
	return UNKNOWN
}

/*
sync.Map implementation:

goos: linux
goarch: amd64
pkg: github.com/go-sqlex/sqlex
BenchmarkBindSpeed/old-4         	100000000	        11.0 ns/op
BenchmarkBindSpeed/new-4         	24575726	        50.8 ns/op


async.Value map implementation:

goos: linux
goarch: amd64
pkg: github.com/go-sqlex/sqlex
BenchmarkBindSpeed/old-4         	100000000	        11.0 ns/op
BenchmarkBindSpeed/new-4         	42535839	        27.5 ns/op
*/

func BenchmarkBindSpeed(b *testing.B) {
	testDrivers := []string{
		"postgres", "pgx", "mysql", "sqlite3", "ora", "sqlserver",
	}

	b.Run("old", func(b *testing.B) {
		b.StopTimer()
		var seq []int
		for i := 0; i < b.N; i++ {
			seq = append(seq, rand.Intn(len(testDrivers)))
		}
		b.StartTimer()
		for i := 0; i < b.N; i++ {
			s := oldBindType(testDrivers[seq[i]])
			if s == UNKNOWN {
				b.Error("unknown driver")
			}
		}

	})

	b.Run("new", func(b *testing.B) {
		b.StopTimer()
		var seq []int
		for i := 0; i < b.N; i++ {
			seq = append(seq, rand.Intn(len(testDrivers)))
		}
		b.StartTimer()
		for i := 0; i < b.N; i++ {
			s := BindType(testDrivers[seq[i]])
			if s == UNKNOWN {
				b.Error("unknown driver")
			}
		}

	})
}

func TestRebindStringLiteral(t *testing.T) {
	tests := []struct {
		name     string
		query    string
		bindType int
		expected string
	}{
		{
			name:     "question mark in string literal not replaced",
			query:    "SELECT * FROM t WHERE name = ? AND desc LIKE '%test?%'",
			bindType: DOLLAR,
			expected: "SELECT * FROM t WHERE name = $1 AND desc LIKE '%test?%'",
		},
		{
			name:     "string literal contains SQL standard escaped quote",
			query:    "SELECT * FROM t WHERE name = ? AND val = 'it''s a test?'",
			bindType: DOLLAR,
			expected: "SELECT * FROM t WHERE name = $1 AND val = 'it''s a test?'",
		},
		{
			name:     "multiple string literals and multiple placeholders mixed",
			query:    "SELECT * FROM t WHERE a = ? AND b = 'hello?' AND c = ? AND d = 'world?'",
			bindType: DOLLAR,
			expected: "SELECT * FROM t WHERE a = $1 AND b = 'hello?' AND c = $2 AND d = 'world?'",
		},
		{
			name:     "empty string literal",
			query:    "SELECT * FROM t WHERE a = '' AND b = ?",
			bindType: DOLLAR,
			expected: "SELECT * FROM t WHERE a = '' AND b = $1",
		},
		{
			name:     "only question mark in string literal",
			query:    "SELECT * FROM t WHERE a = '?' AND b = ?",
			bindType: DOLLAR,
			expected: "SELECT * FROM t WHERE a = '?' AND b = $1",
		},
		{
			name:     "string literal and backslash-escaped question mark coexist",
			query:    "SELECT * FROM t WHERE a = '?' AND b = \\? AND c = ?",
			bindType: DOLLAR,
			expected: "SELECT * FROM t WHERE a = '?' AND b = ? AND c = $1",
		},
		{
			name:     "string literal and double question mark escape coexist",
			query:    "SELECT * FROM t WHERE a = '?' AND b = ?? AND c = ?",
			bindType: DOLLAR,
			expected: "SELECT * FROM t WHERE a = '?' AND b = ? AND c = $1",
		},
		{
			name:     "AT bind type also correctly skips string literal",
			query:    "SELECT * FROM t WHERE a = '?' AND b = ?",
			bindType: AT,
			expected: "SELECT * FROM t WHERE a = '?' AND b = @p1",
		},
		{
			name:     "NAMED bind type also correctly skips string literal",
			query:    "SELECT * FROM t WHERE a = '?' AND b = ?",
			bindType: NAMED,
			expected: "SELECT * FROM t WHERE a = '?' AND b = :arg1",
		},
		{
			name:     "QUESTION type does not replace anything",
			query:    "SELECT * FROM t WHERE a = '?' AND b = ?",
			bindType: QUESTION,
			expected: "SELECT * FROM t WHERE a = '?' AND b = ?",
		},
		{
			name:     "multiple consecutive escaped quotes",
			query:    "SELECT * FROM t WHERE a = ? AND b = '''' AND c = ?",
			bindType: DOLLAR,
			expected: "SELECT * FROM t WHERE a = $1 AND b = '''' AND c = $2",
		},
		{
			name:     "string literal at end of query",
			query:    "SELECT * FROM t WHERE a = ? AND b = 'test?'",
			bindType: DOLLAR,
			expected: "SELECT * FROM t WHERE a = $1 AND b = 'test?'",
		},
		{
			name:     "LIKE wildcard scenario",
			query:    "SELECT * FROM t WHERE name LIKE '%test?%' AND id = ?",
			bindType: DOLLAR,
			expected: "SELECT * FROM t WHERE name LIKE '%test?%' AND id = $1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Rebind(tt.bindType, tt.query)
			if got != tt.expected {
				t.Errorf("Rebind(%d, %q) =\n  %q\nwant:\n  %q", tt.bindType, tt.query, got, tt.expected)
			}
		})
	}
}

// TestRebindIdempotent — P3-7: verify Rebind(Rebind(query)) == Rebind(query)
func TestRebindIdempotent(t *testing.T) {
	queries := []string{
		"SELECT * FROM t WHERE a = ? AND b = ?",
		"SELECT * FROM t WHERE a = ? AND b = 'test?' AND c = ?",
		"INSERT INTO t (a, b, c) VALUES (?, ?, ?)",
		"UPDATE t SET a = ? WHERE b = ? AND c LIKE '%test?%'",
	}

	bindTypes := []struct {
		name     string
		bindType int
	}{
		{"DOLLAR", DOLLAR},
		{"NAMED", NAMED},
		{"AT", AT},
		{"QUESTION", QUESTION},
		{"UNKNOWN", UNKNOWN},
	}

	for _, bt := range bindTypes {
		for _, q := range queries {
			once := Rebind(bt.bindType, q)
			twice := Rebind(bt.bindType, once)
			if once != twice {
				t.Errorf("Rebind not idempotent for %s:\n  query:  %q\n  once:   %q\n  twice:  %q",
					bt.name, q, once, twice)
			}
		}
	}
}
