package sqlex

import (
	"database/sql"
	"errors"
	"testing"
)

// BenchmarkRebindPerf benchmarks Rebind performance
func BenchmarkRebindPerf(b *testing.B) {
	queries := []string{
		"SELECT * FROM t WHERE a = ?",
		"SELECT * FROM t WHERE a = ? AND b = ? AND c = ?",
		"INSERT INTO t (a, b, c, d, e) VALUES (?, ?, ?, ?, ?)",
		"UPDATE t SET a = ?, b = ?, c = ? WHERE d = ? AND e = ?",
		"SELECT * FROM t WHERE a = ? AND b = 'test?' AND c = ?",
	}

	b.Run("DOLLAR", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			Rebind(DOLLAR, queries[i%len(queries)])
		}
	})

	b.Run("QUESTION_noop", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			Rebind(QUESTION, queries[i%len(queries)])
		}
	})
}

// BenchmarkInExpansion benchmarks IN clause expansion performance
func BenchmarkInExpansion(b *testing.B) {
	query := "SELECT * FROM t WHERE id IN (?) AND status = ?"

	b.Run("small_slice_5", func(b *testing.B) {
		ids := []int{1, 2, 3, 4, 5}
		for i := 0; i < b.N; i++ {
			In(query, ids, "active")
		}
	})

	b.Run("medium_slice_100", func(b *testing.B) {
		ids := make([]int, 100)
		for i := range ids {
			ids[i] = i
		}
		for i := 0; i < b.N; i++ {
			In(query, ids, "active")
		}
	})

	b.Run("no_slice", func(b *testing.B) {
		q := "SELECT * FROM t WHERE a = ? AND b = ?"
		for i := 0; i < b.N; i++ {
			In(q, 1, "active")
		}
	})
}

// BenchmarkCompileNamedQuery benchmarks named parameter compilation performance
func BenchmarkCompileNamedQuery(b *testing.B) {
	queries := []struct {
		name  string
		query string
	}{
		{"simple", "SELECT * FROM t WHERE id = :id"},
		{"multi_param", "SELECT * FROM t WHERE a = :a AND b = :b AND c = :c AND d = :d"},
		{"with_string_literal", "SELECT * FROM t WHERE a = :a AND b = 'hello :world' AND c = :c"},
		{"with_pg_cast", "SELECT * FROM t WHERE a::text = :a AND b = :b"},
	}

	for _, tt := range queries {
		b.Run(tt.name, func(b *testing.B) {
			qs := []byte(tt.query)
			for i := 0; i < b.N; i++ {
				compileNamedQuery(qs, QUESTION)
			}
		})
	}
}

// BenchmarkAutoIn benchmarks the zero-overhead fast path of autoIn
func BenchmarkAutoIn(b *testing.B) {
	query := "SELECT * FROM t WHERE a = ? AND b = ? AND c = ?"

	b.Run("no_slice_fast_path", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			autoIn(query, 1, "hello", 3.14)
		}
	})

	b.Run("with_slice", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			autoIn("SELECT * FROM t WHERE id IN (?) AND b = ?", []int{1, 2, 3}, "active")
		}
	})
}

// BenchmarkTopLevelSelect_NoAutoIn_Overhead verifies Phase 2.0:
// top-level Select no longer performs autoIn; when calling noAutoInQueryer,
// query/args are passed through directly, so compared to before the change
// there should be only 1 autoIn call (in the underlying Queryer implementation),
// not 2.
//
// This bench directly calls the top-level Select(noAutoInQueryer);
// since the mock does not perform autoIn, the measured overhead
// is that of the top-level Select itself — should be close to zero.
func BenchmarkTopLevelSelect_NoAutoIn_Overhead(b *testing.B) {
	q := &noAutoInBenchQueryer{}
	dest := &[]struct{}{}

	b.Run("no_slice", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			_ = Select(q, dest, "SELECT 1", 42)
		}
	})

	b.Run("with_slice_passthrough", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			_ = Select(q, dest, "SELECT * FROM t WHERE id IN (?)", []int{1, 2, 3})
		}
	})
}

// noAutoInBenchQueryer shares the same origin as noAutoInQueryer in the contract test,
// but avoids naming conflicts (different test files), and Queryx returns nil rows
// to let Select exit early.
type noAutoInBenchQueryer struct{}

func (q *noAutoInBenchQueryer) Query(query string, args ...any) (*sql.Rows, error) {
	return nil, errBenchNotImpl
}

func (q *noAutoInBenchQueryer) Queryx(query string, args ...any) (*Rows, error) {
	return nil, errBenchNotImpl
}

func (q *noAutoInBenchQueryer) QueryRowx(query string, args ...any) *Row {
	return &Row{err: errBenchNotImpl}
}

var errBenchNotImpl = errors.New("noAutoInBenchQueryer: bench mock")
