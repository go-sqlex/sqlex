package sqlex

import (
	"context"
	"database/sql"

	"github.com/go-sqlex/sqlex/reflectx"
)

// Conn is a wrapper around sql.Conn with extra functionality
type Conn struct {
	*sql.Conn
	driverName string
	Mapper     *reflectx.Mapper
	hooks      []Hook
	strict bool
}

// DriverName returns the driverName used by the DB which created this Conn.
func (c *Conn) DriverName() string {
	return c.driverName
}

// GetMapper returns the Mapper for this Conn.
func (c *Conn) GetMapper() *reflectx.Mapper {
	return c.Mapper
}

// SetStrict enables or disables strict mode.
// In strict mode (true), an error is returned if the query result contains columns
// that have no corresponding field in the target struct.
// In lenient mode (false), mismatched columns are silently ignored.
func (c *Conn) SetStrict(strict bool) {
	c.strict = strict
}

// IsStrict returns whether strict mode is currently enabled.
func (c *Conn) IsStrict() bool {
	return c.strict
}

// getHooks returns the Hook chain of the current Conn, used by Preparex and other
// factory functions to propagate to Stmt.
func (c *Conn) getHooks() []Hook {
	return c.hooks
}

// Rebind a query within a Conn's bindvar type.
func (c *Conn) Rebind(query string) string {
	return Rebind(BindType(c.driverName), query)
}

// BindNamed binds a query within a Conn's bindvar type.
func (c *Conn) BindNamed(query string, arg any) (string, []any, error) {
	return bindNamedMapper(BindType(c.driverName), query, arg, c.Mapper)
}

// BeginTxx begins a transaction and returns an *sqlex.Tx instead of an
// *sql.Tx.
func (c *Conn) BeginTxx(ctx context.Context, opts *sql.TxOptions) (*Tx, error) {
	tx, err := c.Conn.BeginTx(ctx, opts)
	if err != nil {
		return nil, err
	}
	return &Tx{Tx: tx, driverName: c.driverName, Mapper: c.Mapper, hooks: c.hooks, strict: c.strict}, err
}

// SelectContext using this Conn.
// Any placeholder parameters are replaced with supplied args.
func (c *Conn) SelectContext(ctx context.Context, dest any, query string, args ...any) error {
	return SelectContext(ctx, c, dest, query, args...)
}

// GetContext using this Conn.
// Any placeholder parameters are replaced with supplied args.
// An error is returned if the result set is empty.
func (c *Conn) GetContext(ctx context.Context, dest any, query string, args ...any) error {
	return GetContext(ctx, c, dest, query, args...)
}

// PreparexContext returns an sqlex.Stmt instead of a sql.Stmt.
func (c *Conn) PreparexContext(ctx context.Context, query string) (*Stmt, error) {
	return PreparexContext(ctx, c, query)
}

// QueryxContext queries the database and returns an *sqlex.Rows.
// Any placeholder parameters are replaced with supplied args.
// Automatically detects slice args and expands IN clauses; zero overhead when no slices are present.
func (c *Conn) QueryxContext(ctx context.Context, query string, args ...any) (*Rows, error) {
	var err error
	if query, args, err = autoIn(query, args...); err != nil {
		return nil, err
	}
	query = Rebind(BindType(c.driverName), query)
	event := &QueryEvent{Query: query, Args: args, OperationType: "query"}
	ctx, afterFunc := executeHooks(ctx, c.hooks, event)
	r, err := c.Conn.QueryContext(ctx, query, args...)
	event.Error = err
	afterFunc()
	if err != nil {
		return nil, err
	}
	return &Rows{Rows: r, Mapper: c.Mapper, strict: c.strict}, nil
}

// QueryRowxContext queries the database and returns an *sqlex.Row.
// Any placeholder parameters are replaced with supplied args.
// Automatically detects slice args and expands IN clauses; zero overhead when no slices are present.
func (c *Conn) QueryRowxContext(ctx context.Context, query string, args ...any) *Row {
	var err error
	if query, args, err = autoIn(query, args...); err != nil {
		return &Row{err: err, Mapper: c.Mapper, strict: c.strict}
	}
	query = Rebind(BindType(c.driverName), query)
	event := &QueryEvent{Query: query, Args: args, OperationType: "query"}
	ctx, afterFunc := executeHooks(ctx, c.hooks, event)
	rows, err := c.Conn.QueryContext(ctx, query, args...)
	event.Error = err
	afterFunc()
	return &Row{rows: rows, err: err, Mapper: c.Mapper, strict: c.strict}
}

// NamedQueryContext using this Conn.
// Any named placeholder parameters are replaced with fields from arg.
func (c *Conn) NamedQueryContext(ctx context.Context, query string, arg any) (*Rows, error) {
	return NamedQueryContext(ctx, c, query, arg)
}

// ExecContext executes a query without returning any rows.
// Overrides sql.Conn's ExecContext to integrate Hook logic.
// Automatically detects slice args and expands IN clauses; zero overhead when no slices are present.
func (c *Conn) ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error) {
	var err error
	if query, args, err = autoIn(query, args...); err != nil {
		return nil, err
	}
	query = Rebind(BindType(c.driverName), query)
	event := &QueryEvent{Query: query, Args: args, OperationType: "exec"}
	ctx, afterFunc := executeHooks(ctx, c.hooks, event)
	result, err := c.Conn.ExecContext(ctx, query, args...)
	event.Error = err
	afterFunc()
	return result, err
}

// NamedExecContext using this Conn.
// Any named placeholder parameters are replaced with fields from arg.
//
// Note: This method only does named-to-? conversion and forwarding; autoIn / Rebind / Hook
// are guaranteed by the downstream c.ExecContext implementation (see ExecerContext interface contract).
func (c *Conn) NamedExecContext(ctx context.Context, query string, arg any) (sql.Result, error) {
	q, args, err := bindNamedMapper(QUESTION, query, arg, c.Mapper)
	if err != nil {
		return nil, err
	}
	return c.ExecContext(ctx, q, args...)
}

// PrepareNamedContext returns an sqlex.NamedStmt
func (c *Conn) PrepareNamedContext(ctx context.Context, query string) (*NamedStmt, error) {
	return prepareNamedContext(ctx, c, query)
}

// MustExecContext (panic) runs MustExec using this Conn.
// Any placeholder parameters are replaced with supplied args.
func (c *Conn) MustExecContext(ctx context.Context, query string, args ...any) sql.Result {
	return MustExecContext(ctx, c, query, args...)
}

// --- Non-Context convenience methods (aligned with DB/Tx, making Conn satisfy NamedExt, BindExt, and Ext interfaces) ---

// Query executes a query that returns rows.
// Any placeholder parameters are replaced with supplied args.
func (c *Conn) Query(query string, args ...any) (*sql.Rows, error) {
	return c.Conn.QueryContext(context.Background(), query, args...)
}

// Prepare creates a prepared statement for later queries or executions.
func (c *Conn) Prepare(query string) (*sql.Stmt, error) {
	return c.Conn.PrepareContext(context.Background(), query)
}

// Select using this Conn.
// Any placeholder parameters are replaced with supplied args.
func (c *Conn) Select(dest any, query string, args ...any) error {
	return c.SelectContext(context.Background(), dest, query, args...)
}

// Get using this Conn.
// Any placeholder parameters are replaced with supplied args.
// An error is returned if the result set is empty.
func (c *Conn) Get(dest any, query string, args ...any) error {
	return c.GetContext(context.Background(), dest, query, args...)
}

// Queryx queries the database and returns an *sqlex.Rows.
// Any placeholder parameters are replaced with supplied args.
func (c *Conn) Queryx(query string, args ...any) (*Rows, error) {
	return c.QueryxContext(context.Background(), query, args...)
}

// QueryRowx queries the database and returns an *sqlex.Row.
// Any placeholder parameters are replaced with supplied args.
func (c *Conn) QueryRowx(query string, args ...any) *Row {
	return c.QueryRowxContext(context.Background(), query, args...)
}

// Exec executes a query without returning any rows.
// Any placeholder parameters are replaced with supplied args.
func (c *Conn) Exec(query string, args ...any) (sql.Result, error) {
	return c.ExecContext(context.Background(), query, args...)
}

// MustExec (panic) runs MustExec using this Conn.
// Any placeholder parameters are replaced with supplied args.
func (c *Conn) MustExec(query string, args ...any) sql.Result {
	return MustExec(c, query, args...)
}

// NamedQuery using this Conn.
// Any named placeholder parameters are replaced with fields from arg.
func (c *Conn) NamedQuery(query string, arg any) (*Rows, error) {
	return NamedQuery(c, query, arg)
}

// NamedExec using this Conn.
// Any named placeholder parameters are replaced with fields from arg.
// Automatically detects slice args and expands IN clauses; zero overhead when no slices are present.
func (c *Conn) NamedExec(query string, arg any) (sql.Result, error) {
	return c.NamedExecContext(context.Background(), query, arg)
}

// NamedGet executes a query with named parameters and scans a single row result.
// Automatically detects slice args and expands IN clauses; zero overhead when no slices are present.
func (c *Conn) NamedGet(dest any, query string, param any) error {
	return c.NamedGetContext(context.Background(), dest, query, param)
}

// NamedSelect executes a query with named parameters and scans the result set into a slice.
// Automatically detects slice args and expands IN clauses; zero overhead when no slices are present.
func (c *Conn) NamedSelect(dest any, query string, param any) error {
	return c.NamedSelectContext(context.Background(), dest, query, param)
}
