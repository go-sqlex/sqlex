package sqlex

import (
	"context"
	"database/sql"
	"fmt"
	"reflect"

	"github.com/go-sqlex/sqlex/reflectx"
)

// Tx is sqlex's enhanced wrapper around sql.Tx.
// Note: Tx, like database/sql.Tx, is not safe for concurrent use.
// Do not share the same Tx instance across goroutines for concurrent queries.
type Tx struct {
	*sql.Tx
	driverName string
	Mapper     *reflectx.Mapper
	hooks      []Hook
	strict     bool
}

// DriverName returns the driverName used by the DB which began this transaction.
func (tx *Tx) DriverName() string {
	return tx.driverName
}

// GetMapper returns the Mapper for this Tx.
func (tx *Tx) GetMapper() *reflectx.Mapper {
	return tx.Mapper
}

// SetStrict enables or disables strict mode.
// In strict mode (true), an error is returned if the query result contains columns
// that have no corresponding field in the target struct.
// In lenient mode (false), mismatched columns are silently ignored.
func (tx *Tx) SetStrict(strict bool) {
	tx.strict = strict
}

// IsStrict returns whether strict mode is currently enabled.
func (tx *Tx) IsStrict() bool {
	return tx.strict
}

// getHooks returns the Hook chain of the current Tx, used by Preparex and other
// factory functions to propagate to Stmt.
func (tx *Tx) getHooks() []Hook {
	return tx.hooks
}

// Rebind a query within a transaction's bindvar type.
func (tx *Tx) Rebind(query string) string {
	return Rebind(BindType(tx.driverName), query)
}

// BindNamed binds a query within a transaction's bindvar type.
func (tx *Tx) BindNamed(query string, arg any) (string, []any, error) {
	return bindNamedMapper(BindType(tx.driverName), query, arg, tx.Mapper)
}

// NamedQuery within a transaction.
// Any named placeholder parameters are replaced with fields from arg.
func (tx *Tx) NamedQuery(query string, arg any) (*Rows, error) {
	return NamedQuery(tx, query, arg)
}

// NamedExec a named query within a transaction.
// Any named placeholder parameters are replaced with fields from arg.
// Automatically detects slice args and expands IN clauses; zero overhead when no slices are present.
func (tx *Tx) NamedExec(query string, arg any) (sql.Result, error) {
	return tx.NamedExecContext(context.Background(), query, arg)
}

// Select within a transaction.
// Any placeholder parameters are replaced with supplied args.
func (tx *Tx) Select(dest any, query string, args ...any) error {
	return Select(tx, dest, query, args...)
}

// Queryx within a transaction.
// Any placeholder parameters are replaced with supplied args.
func (tx *Tx) Queryx(query string, args ...any) (*Rows, error) {
	return tx.QueryxContext(context.Background(), query, args...)
}

// QueryRowx within a transaction.
// Any placeholder parameters are replaced with supplied args.
func (tx *Tx) QueryRowx(query string, args ...any) *Row {
	return tx.QueryRowxContext(context.Background(), query, args...)
}

// Get within a transaction.
// Any placeholder parameters are replaced with supplied args.
// An error is returned if the result set is empty.
func (tx *Tx) Get(dest any, query string, args ...any) error {
	return Get(tx, dest, query, args...)
}

// Exec executes a query within a transaction without returning any rows.
// Any placeholder parameters are replaced with supplied args.
// Overrides sql.Tx's Exec, delegating to ExecContext to integrate Hook logic.
func (tx *Tx) Exec(query string, args ...any) (sql.Result, error) {
	return tx.ExecContext(context.Background(), query, args...)
}

// MustExec runs MustExec within a transaction.
// Any placeholder parameters are replaced with supplied args.
func (tx *Tx) MustExec(query string, args ...any) sql.Result {
	return MustExec(tx, query, args...)
}

// Preparex  a statement within a transaction.
func (tx *Tx) Preparex(query string) (*Stmt, error) {
	return Preparex(tx, query)
}

// Stmtx returns a version of the prepared statement which runs within a transaction.  Provided
// stmt can be either *sql.Stmt or *sqlex.Stmt.
//
// WARNING: Stmtx will panic if stmt is not a valid type. Use TryStmtx for a safe version.
func (tx *Tx) Stmtx(stmt any) *Stmt {
	var s *sql.Stmt
	switch v := stmt.(type) {
	case Stmt:
		s = v.Stmt
	case *Stmt:
		s = v.Stmt
	case *sql.Stmt:
		s = v
	default:
		panic(fmt.Sprintf("non-statement type %v passed to Stmtx", reflect.ValueOf(stmt).Type()))
	}
	return &Stmt{Stmt: tx.Stmt(s), Mapper: tx.Mapper, hooks: tx.hooks, strict: tx.strict}
}

// TryStmtx is a safe version of Stmtx that returns an error instead of panicking.
func (tx *Tx) TryStmtx(stmt any) (*Stmt, error) {
	var s *sql.Stmt
	switch v := stmt.(type) {
	case Stmt:
		s = v.Stmt
	case *Stmt:
		s = v.Stmt
	case *sql.Stmt:
		s = v
	default:
		return nil, fmt.Errorf("non-statement type %v passed to TryStmtx", reflect.ValueOf(stmt).Type())
	}
	return &Stmt{Stmt: tx.Stmt(s), Mapper: tx.Mapper, hooks: tx.hooks, strict: tx.strict}, nil
}

// StmtxContext returns a version of the prepared statement which runs within a
// transaction. Provided stmt can be either *sql.Stmt or *sqlex.Stmt.
//
// WARNING: StmtxContext will panic if stmt is not a valid type. Use TryStmtxContext for a safe version.
func (tx *Tx) StmtxContext(ctx context.Context, stmt any) *Stmt {
	var s *sql.Stmt
	switch v := stmt.(type) {
	case Stmt:
		s = v.Stmt
	case *Stmt:
		s = v.Stmt
	case *sql.Stmt:
		s = v
	default:
		panic(fmt.Sprintf("non-statement type %v passed to StmtxContext", reflect.ValueOf(stmt).Type()))
	}
	return &Stmt{Stmt: tx.StmtContext(ctx, s), Mapper: tx.Mapper, hooks: tx.hooks, strict: tx.strict}
}

// TryStmtxContext is a safe version of StmtxContext that returns an error instead of panicking.
func (tx *Tx) TryStmtxContext(ctx context.Context, stmt any) (*Stmt, error) {
	var s *sql.Stmt
	switch v := stmt.(type) {
	case Stmt:
		s = v.Stmt
	case *Stmt:
		s = v.Stmt
	case *sql.Stmt:
		s = v
	default:
		return nil, fmt.Errorf("non-statement type %v passed to TryStmtxContext", reflect.ValueOf(stmt).Type())
	}
	return &Stmt{Stmt: tx.StmtContext(ctx, s), Mapper: tx.Mapper, hooks: tx.hooks, strict: tx.strict}, nil
}

// NamedStmt returns a version of the prepared statement which runs within a transaction.
func (tx *Tx) NamedStmt(stmt *NamedStmt) *NamedStmt {
	return &NamedStmt{
		QueryString: stmt.QueryString,
		Params:      stmt.Params,
		Stmt:        tx.Stmtx(stmt.Stmt),
		strict:      tx.strict,
	}
}

// NamedStmtContext returns a version of the prepared statement which runs
// within a transaction.
func (tx *Tx) NamedStmtContext(ctx context.Context, stmt *NamedStmt) *NamedStmt {
	return &NamedStmt{
		QueryString: stmt.QueryString,
		Params:      stmt.Params,
		Stmt:        tx.StmtxContext(ctx, stmt.Stmt),
		strict:      tx.strict,
	}
}

// PrepareNamed returns an sqlex.NamedStmt
func (tx *Tx) PrepareNamed(query string) (*NamedStmt, error) {
	return prepareNamed(tx, query)
}

// PrepareNamedContext returns an sqlex.NamedStmt
func (tx *Tx) PrepareNamedContext(ctx context.Context, query string) (*NamedStmt, error) {
	return prepareNamedContext(ctx, tx, query)
}

// PreparexContext returns an sqlex.Stmt instead of a sql.Stmt.
func (tx *Tx) PreparexContext(ctx context.Context, query string) (*Stmt, error) {
	return PreparexContext(ctx, tx, query)
}

// MustExecContext runs MustExecContext within a transaction.
// Any placeholder parameters are replaced with supplied args.
func (tx *Tx) MustExecContext(ctx context.Context, query string, args ...any) sql.Result {
	return MustExecContext(ctx, tx, query, args...)
}

// QueryxContext within a transaction and context.
// Any placeholder parameters are replaced with supplied args.
// Automatically detects slice args and expands IN clauses; zero overhead when no slices are present.
func (tx *Tx) QueryxContext(ctx context.Context, query string, args ...any) (*Rows, error) {
	var err error
	if query, args, err = autoIn(query, args...); err != nil {
		return nil, err
	}
	query = Rebind(BindType(tx.driverName), query)
	event := &QueryEvent{Query: query, Args: args, OperationType: "query"}
	ctx, afterFunc := executeHooks(ctx, tx.hooks, event)
	r, err := tx.Tx.QueryContext(ctx, query, args...)
	event.Error = err
	afterFunc()
	if err != nil {
		return nil, err
	}
	return &Rows{Rows: r, Mapper: tx.Mapper, strict: tx.strict}, nil
}

// SelectContext within a transaction and context.
// Any placeholder parameters are replaced with supplied args.
func (tx *Tx) SelectContext(ctx context.Context, dest any, query string, args ...any) error {
	return SelectContext(ctx, tx, dest, query, args...)
}

// GetContext within a transaction and context.
// Any placeholder parameters are replaced with supplied args.
// An error is returned if the result set is empty.
func (tx *Tx) GetContext(ctx context.Context, dest any, query string, args ...any) error {
	return GetContext(ctx, tx, dest, query, args...)
}

// QueryRowxContext within a transaction and context.
// Any placeholder parameters are replaced with supplied args.
// Automatically detects slice args and expands IN clauses; zero overhead when no slices are present.
func (tx *Tx) QueryRowxContext(ctx context.Context, query string, args ...any) *Row {
	var err error
	if query, args, err = autoIn(query, args...); err != nil {
		return &Row{err: err, Mapper: tx.Mapper, strict: tx.strict}
	}
	query = Rebind(BindType(tx.driverName), query)
	event := &QueryEvent{Query: query, Args: args, OperationType: "query"}
	ctx, afterFunc := executeHooks(ctx, tx.hooks, event)
	rows, err := tx.Tx.QueryContext(ctx, query, args...)
	event.Error = err
	afterFunc()
	return &Row{rows: rows, err: err, Mapper: tx.Mapper, strict: tx.strict}
}

// NamedQueryContext within a transaction and context.
// Any named placeholder parameters are replaced with fields from arg.
func (tx *Tx) NamedQueryContext(ctx context.Context, query string, arg any) (*Rows, error) {
	return NamedQueryContext(ctx, tx, query, arg)
}

// ExecContext executes a query without returning any rows.
// Overrides sql.Tx's ExecContext to integrate Hook logic.
// Automatically detects slice args and expands IN clauses; zero overhead when no slices are present.
func (tx *Tx) ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error) {
	var err error
	if query, args, err = autoIn(query, args...); err != nil {
		return nil, err
	}
	query = Rebind(BindType(tx.driverName), query)
	event := &QueryEvent{Query: query, Args: args, OperationType: "exec"}
	ctx, afterFunc := executeHooks(ctx, tx.hooks, event)
	result, err := tx.Tx.ExecContext(ctx, query, args...)
	event.Error = err
	afterFunc()
	return result, err
}

// NamedExecContext using this Tx.
// Any named placeholder parameters are replaced with fields from arg.
//
// Note: This method only does named-to-? conversion and forwarding; autoIn / Rebind / Hook
// are guaranteed by the downstream tx.ExecContext implementation (see ExecerContext interface contract).
func (tx *Tx) NamedExecContext(ctx context.Context, query string, arg any) (sql.Result, error) {
	q, args, err := bindNamedMapper(QUESTION, query, arg, tx.Mapper)
	if err != nil {
		return nil, err
	}
	return tx.ExecContext(ctx, q, args...)
}

// --- Enhanced transaction management methods (originally tx_ext.go) ---

// CloseWithErr automatically commits or rolls back the transaction based on the error value.
// If err is nil, the transaction is committed; otherwise, it is rolled back.
// Commit/Rollback failures are reported via Hook.
func (tx *Tx) CloseWithErr(err error) {
	if err != nil {
		if rbErr := tx.Rollback(); rbErr != nil {
			tx.logTxError("rollback", rbErr)
		}
	} else {
		if cmErr := tx.Commit(); cmErr != nil {
			tx.logTxError("commit", cmErr)
		}
	}
}

// logTxError reports transaction operation (commit/rollback) failures via Hook.
func (tx *Tx) logTxError(op string, err error) {
	if len(tx.hooks) == 0 {
		return
	}
	event := &QueryEvent{
		Query:         "TX " + op,
		OperationType: op,
		Error:         err,
	}
	ctx := context.Background()
	_, afterFunc := executeHooks(ctx, tx.hooks, event)
	afterFunc()
}
