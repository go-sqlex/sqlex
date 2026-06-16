package sqlex

import (
	"context"
	"database/sql"
	"errors"
	"testing"
)

// ============================================================================
// Phase 2.0 - Interface contract regression tests
//
// Goal: Prove that top-level Select/Get/NamedQuery etc. **no longer** perform autoIn,
// relying entirely on Queryer/Execer implementations to guarantee autoIn.
//
// Approach: Define mock Queryer/Execer that **do not** perform autoIn,
// and verify that when SQL contains IN (?) + slice args, the downstream SQL
// assembly keeps ? as a single placeholder (rather than expanding to ?,?,?)
// — this proves the top-level layer does not have a fallback.
// ============================================================================

// noAutoInQueryer is a Queryer implementation that deliberately does not perform autoIn.
// It records each call for asserting whether autoIn was executed at the top level.
type noAutoInQueryer struct {
	lastQuery string
	lastArgs  []any
}

func (q *noAutoInQueryer) Query(query string, args ...any) (*sql.Rows, error) {
	q.lastQuery = query
	q.lastArgs = args
	return nil, errors.New("noAutoInQueryer.Query: not implemented for unit tests")
}

func (q *noAutoInQueryer) Queryx(query string, args ...any) (*Rows, error) {
	q.lastQuery = query
	q.lastArgs = args
	return nil, errors.New("noAutoInQueryer.Queryx: not implemented for unit tests")
}

func (q *noAutoInQueryer) QueryRowx(query string, args ...any) *Row {
	q.lastQuery = query
	q.lastArgs = args
	return &Row{err: errors.New("noAutoInQueryer.QueryRowx: not implemented for unit tests")}
}

// noAutoInQueryerContext is a mock version of QueryerContext.
type noAutoInQueryerContext struct {
	lastQuery string
	lastArgs  []any
}

func (q *noAutoInQueryerContext) QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error) {
	q.lastQuery = query
	q.lastArgs = args
	return nil, errors.New("noAutoInQueryerContext.QueryContext: not implemented")
}

func (q *noAutoInQueryerContext) QueryxContext(ctx context.Context, query string, args ...any) (*Rows, error) {
	q.lastQuery = query
	q.lastArgs = args
	return nil, errors.New("noAutoInQueryerContext.QueryxContext: not implemented")
}

func (q *noAutoInQueryerContext) QueryRowxContext(ctx context.Context, query string, args ...any) *Row {
	q.lastQuery = query
	q.lastArgs = args
	return &Row{err: errors.New("noAutoInQueryerContext.QueryRowxContext: not implemented")}
}

// TestContract_TopLevelSelect_DoesNotAutoIn verifies that top-level Select no longer performs autoIn —
// when passed through to Queryer.Queryx, the query and args must be exactly as provided.
func TestContract_TopLevelSelect_DoesNotAutoIn(t *testing.T) {
	q := &noAutoInQueryer{}
	rawQuery := `SELECT * FROM t WHERE id IN (?)`
	rawArgs := []any{[]int{1, 2, 3}}

	// Select returns when Queryx errors, but we care about q.lastQuery / lastArgs
	_ = Select(q, &[]struct{}{}, rawQuery, rawArgs...)

	if q.lastQuery != rawQuery {
		t.Errorf("Top-level Select should not rewrite query:\n  got =%q\n  want=%q", q.lastQuery, rawQuery)
	}
	if len(q.lastArgs) != 1 {
		t.Errorf("Top-level Select should not expand args: got len=%d, want 1", len(q.lastArgs))
	}
	if got, ok := q.lastArgs[0].([]int); !ok || len(got) != 3 {
		t.Errorf("Top-level Select should preserve original slice arg: got %v (%T)", q.lastArgs[0], q.lastArgs[0])
	}
}

// TestContract_TopLevelGet_DoesNotAutoIn verifies that top-level Get no longer performs autoIn.
func TestContract_TopLevelGet_DoesNotAutoIn(t *testing.T) {
	q := &noAutoInQueryer{}
	rawQuery := `SELECT * FROM t WHERE id IN (?)`
	rawArgs := []any{[]int{1, 2, 3}}

	_ = Get(q, &struct{}{}, rawQuery, rawArgs...)

	if q.lastQuery != rawQuery {
		t.Errorf("top-level Get should not rewrite query:\n  got =%q\n  want=%q", q.lastQuery, rawQuery)
	}
	if len(q.lastArgs) != 1 {
		t.Errorf("top-level Get should not expand args: got len=%d, want 1", len(q.lastArgs))
	}
}

// TestContract_TopLevelSelectContext_DoesNotAutoIn verifies SelectContext.
func TestContract_TopLevelSelectContext_DoesNotAutoIn(t *testing.T) {
	q := &noAutoInQueryerContext{}
	rawQuery := `SELECT * FROM t WHERE id IN (?)`
	rawArgs := []any{[]int{1, 2, 3}}

	_ = SelectContext(context.Background(), q, &[]struct{}{}, rawQuery, rawArgs...)

	if q.lastQuery != rawQuery {
		t.Errorf("top-level SelectContext should not rewrite query:\n  got =%q\n  want=%q", q.lastQuery, rawQuery)
	}
	if len(q.lastArgs) != 1 {
		t.Errorf("top-level SelectContext should not expand args: got len=%d, want 1", len(q.lastArgs))
	}
}

// TestContract_TopLevelGetContext_DoesNotAutoIn verifies GetContext.
func TestContract_TopLevelGetContext_DoesNotAutoIn(t *testing.T) {
	q := &noAutoInQueryerContext{}
	rawQuery := `SELECT * FROM t WHERE id IN (?)`
	rawArgs := []any{[]int{1, 2, 3}}

	_ = GetContext(context.Background(), q, &struct{}{}, rawQuery, rawArgs...)

	if q.lastQuery != rawQuery {
		t.Errorf("top-level GetContext should not rewrite query:\n  got =%q\n  want=%q", q.lastQuery, rawQuery)
	}
	if len(q.lastArgs) != 1 {
		t.Errorf("top-level GetContext should not expand args: got len=%d, want 1", len(q.lastArgs))
	}
}
