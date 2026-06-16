package sqlex

import (
	"context"
	"database/sql"

	"github.com/go-sqlex/sqlex/reflectx"
)

// Stmt is an sqlex wrapper around sql.Stmt with extra functionality
type Stmt struct {
	*sql.Stmt
	Mapper     *reflectx.Mapper
	hooks      []Hook
	query      string // Original SQL at prepare time, used for Hook event reporting
	strict bool
}

// GetMapper returns the Mapper for this Stmt.
func (s *Stmt) GetMapper() *reflectx.Mapper {
	return s.Mapper
}

// Exec executes the prepared statement with the given arguments.
func (s *Stmt) Exec(args ...any) (sql.Result, error) {
	return s.ExecContext(context.Background(), args...)
}

// ExecContext executes the prepared statement with the given arguments.
func (s *Stmt) ExecContext(ctx context.Context, args ...any) (sql.Result, error) {
	event := &QueryEvent{Query: s.query, Args: args, OperationType: "exec"}
	ctx, afterFunc := executeHooks(ctx, s.hooks, event)
	result, err := s.Stmt.ExecContext(ctx, args...)
	event.Error = err
	afterFunc()
	return result, err
}

// Query executes the prepared statement with the given arguments.
func (s *Stmt) Query(args ...any) (*sql.Rows, error) {
	return s.QueryContext(context.Background(), args...)
}

// QueryContext executes the prepared statement with the given arguments.
func (s *Stmt) QueryContext(ctx context.Context, args ...any) (*sql.Rows, error) {
	event := &QueryEvent{Query: s.query, Args: args, OperationType: "query"}
	ctx, afterFunc := executeHooks(ctx, s.hooks, event)
	r, err := s.Stmt.QueryContext(ctx, args...)
	event.Error = err
	afterFunc()
	return r, err
}

// Queryx using the prepared statement.
func (s *Stmt) Queryx(args ...any) (*Rows, error) {
	return s.QueryxContext(context.Background(), args...)
}

// QueryxContext using the prepared statement.
func (s *Stmt) QueryxContext(ctx context.Context, args ...any) (*Rows, error) {
	r, err := s.QueryContext(ctx, args...)
	if err != nil {
		return nil, err
	}
	return &Rows{Rows: r, Mapper: s.Mapper, strict: s.strict}, nil
}

// QueryRowx using the prepared statement.
func (s *Stmt) QueryRowx(args ...any) *Row {
	return s.QueryRowxContext(context.Background(), args...)
}

// QueryRowxContext using the prepared statement.
func (s *Stmt) QueryRowxContext(ctx context.Context, args ...any) *Row {
	rows, err := s.QueryContext(ctx, args...)
	return &Row{rows: rows, err: err, Mapper: s.Mapper, strict: s.strict}
}

// Select using the prepared statement.
func (s *Stmt) Select(dest any, args ...any) error {
	return s.SelectContext(context.Background(), dest, args...)
}

// SelectContext using the prepared statement.
func (s *Stmt) SelectContext(ctx context.Context, dest any, args ...any) error {
	rows, err := s.QueryxContext(ctx, args...)
	if err != nil {
		return err
	}
	defer rows.Close()
	return scanAll(rows, dest, false)
}

// Get using the prepared statement.
// An error is returned if the result set is empty.
func (s *Stmt) Get(dest any, args ...any) error {
	return s.GetContext(context.Background(), dest, args...)
}

// GetContext using the prepared statement.
// An error is returned if the result set is empty.
func (s *Stmt) GetContext(ctx context.Context, dest any, args ...any) error {
	r := s.QueryRowxContext(ctx, args...)
	return r.scanAny(dest, false)
}

// MustExec (panic) using this statement.
func (s *Stmt) MustExec(args ...any) sql.Result {
	return s.MustExecContext(context.Background(), args...)
}

// MustExecContext (panic) using this statement.
func (s *Stmt) MustExecContext(ctx context.Context, args ...any) sql.Result {
	res, err := s.ExecContext(ctx, args...)
	if err != nil {
		panic(err)
	}
	return res
}
