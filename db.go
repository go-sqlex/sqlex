package sqlex

import (
	"context"
	"database/sql"

	"github.com/go-sqlex/sqlex/reflectx"
)

// DB is a wrapper around sql.DB which keeps track of the driverName upon Open,
// used mostly to automatically bind named queries using the right bindvars.
type DB struct {
	*sql.DB
	pipeline
	Mapper *reflectx.Mapper
	strict bool
}
type Opt func(db *DB)

// WithHooks returns an Opt that registers Hook chain(s) when creating a DB.
func WithHooks(hooks ...Hook) Opt {
	return func(db *DB) {
		db.hooks = append(db.hooks, hooks...)
	}
}

// WithMapperFunc returns an Opt that sets a custom field mapping function when creating a DB.
func WithMapperFunc(mf func(string) string) Opt {
	return func(db *DB) {
		db.Mapper = reflectx.NewMapperFunc("db", mf)
	}
}

// NewDB returns a new sqlex DB wrapper for a pre-existing *sql.DB.  The
// driverName of the original database is required for named query support.
func NewDB(db *sql.DB, driverName string, opts ...Opt) *DB {
	d := &DB{DB: db, pipeline: pipeline{driverName: driverName}, Mapper: mapper()}
	for _, opt := range opts {
		opt(d)
	}
	return d
}

// DriverName returns the driverName passed to the Open function for this DB.
func (db *DB) DriverName() string {
	return db.driverName
}

// GetMapper returns the Mapper for this DB.
func (db *DB) GetMapper() *reflectx.Mapper {
	return db.Mapper
}

// Open is the same as sql.Open, but returns an *sqlex.DB instead.
func Open(driverName, dataSourceName string, opts ...Opt) (*DB, error) {
	db, err := sql.Open(driverName, dataSourceName)
	if err != nil {
		return nil, err
	}
	d := &DB{DB: db, pipeline: pipeline{driverName: driverName}, Mapper: mapper()}
	for _, opt := range opts {
		opt(d)
	}
	return d, err
}

// MustOpen is the same as sql.Open, but returns an *sqlex.DB instead and panics on error.
func MustOpen(driverName, dataSourceName string, opts ...Opt) *DB {
	db, err := Open(driverName, dataSourceName, opts...)
	if err != nil {
		panic(err)
	}
	return db
}

// SetStrict enables or disables strict mode.
// In strict mode (true), an error is returned if the query result contains columns
// that have no corresponding field in the target struct.
// In lenient mode (false, default), mismatched columns are silently ignored.
func (db *DB) SetStrict(strict bool) {
	db.strict = strict
}

// IsStrict returns whether strict mode is currently enabled.
func (db *DB) IsStrict() bool {
	return db.strict
}

// MapperFunc sets a new mapper for this db using the default sqlex struct tag
// and the provided mapper function.
func (db *DB) MapperFunc(mf func(string) string) {
	db.Mapper = reflectx.NewMapperFunc("db", mf)
}

// Rebind transforms a query from QUESTION to the DB driver's bindvar type.
func (db *DB) Rebind(query string) string {
	return Rebind(BindType(db.driverName), query)
}

// AddHook registers a Hook instance that applies to all queries on this DB.
func (db *DB) AddHook(hook Hook) {
	db.hooks = append(db.hooks, hook)
}

// getHooks returns the Hook chain of the current DB, used by Preparex and other
// factory functions to propagate to Stmt.
func (db *DB) getHooks() []Hook {
	return db.hooks
}

// BindNamed binds a query using the DB driver's bindvar type.
func (db *DB) BindNamed(query string, arg any) (string, []any, error) {
	return bindNamedMapper(BindType(db.driverName), query, arg, db.Mapper)
}

// NamedQuery using this DB.
// Any named placeholder parameters are replaced with fields from arg.
func (db *DB) NamedQuery(query string, arg any) (*Rows, error) {
	return NamedQuery(db, query, arg)
}

// NamedExec using this DB.
// Any named placeholder parameters are replaced with fields from arg.
// Automatically detects slice args and expands IN clauses; zero overhead when no slices are present.
func (db *DB) NamedExec(query string, arg any) (sql.Result, error) {
	return db.NamedExecContext(context.Background(), query, arg)
}

// Select using this DB.
// Any placeholder parameters are replaced with supplied args.
func (db *DB) Select(dest any, query string, args ...any) error {
	return Select(db, dest, query, args...)
}

// Get using this DB.
// Any placeholder parameters are replaced with supplied args.
// An error is returned if the result set is empty.
func (db *DB) Get(dest any, query string, args ...any) error {
	return Get(db, dest, query, args...)
}

// MustBegin starts a transaction, and panics on error.  Returns an *sqlex.Tx instead
// of an *sql.Tx.
func (db *DB) MustBegin() *Tx {
	tx, err := db.Beginx()
	if err != nil {
		panic(err)
	}
	return tx
}

// Beginx begins a transaction and returns an *sqlex.Tx instead of an *sql.Tx.
func (db *DB) Beginx() (*Tx, error) {
	_, afterFunc, event, err := db.prepare(context.Background(), "", nil, OpBegin)
	if err != nil {
		return nil, err
	}
	tx, txErr := db.DB.Begin()
	event.Error = txErr
	afterFunc()
	if txErr != nil {
		return nil, txErr
	}
	return &Tx{Tx: tx, pipeline: db.pipeline, Mapper: db.Mapper, strict: db.strict}, nil
}

// MustBeginTxx starts a transaction, and panics on error.  Returns an *sqlex.Tx instead
// of an *sql.Tx.
//
// The provided context is used until the transaction is committed or rolled
// back. If the context is canceled, the sql package will roll back the
// transaction. Tx.Commit will return an error if the context provided to
// MustBeginContext is canceled.
func (db *DB) MustBeginTxx(ctx context.Context, opts *sql.TxOptions) *Tx {
	tx, err := db.BeginTxx(ctx, opts)
	if err != nil {
		panic(err)
	}
	return tx
}

// BeginTxx begins a transaction and returns an *sqlex.Tx instead of an
// *sql.Tx.
//
// The provided context is used until the transaction is committed or rolled
// back. If the context is canceled, the sql package will roll back the
// transaction. Tx.Commit will return an error if the context provided to
// BeginTxx is canceled.
func (db *DB) BeginTxx(ctx context.Context, opts *sql.TxOptions) (*Tx, error) {
	ctx, afterFunc, event, err := db.prepare(ctx, "", nil, OpBegin)
	if err != nil {
		return nil, err
	}
	tx, txErr := db.DB.BeginTx(ctx, opts)
	event.Error = txErr
	afterFunc()
	if txErr != nil {
		return nil, txErr
	}
	return &Tx{Tx: tx, pipeline: db.pipeline, Mapper: db.Mapper, strict: db.strict}, nil
}

// Queryx queries the database and returns an *sqlex.Rows.
// Any placeholder parameters are replaced with supplied args.
func (db *DB) Queryx(query string, args ...any) (*Rows, error) {
	return db.QueryxContext(context.Background(), query, args...)
}

// QueryRowx queries the database and returns an *sqlex.Row.
// Any placeholder parameters are replaced with supplied args.
func (db *DB) QueryRowx(query string, args ...any) *Row {
	return db.QueryRowxContext(context.Background(), query, args...)
}

// Exec executes a query without returning any rows.
// Any placeholder parameters are replaced with supplied args.
// Overrides sql.DB's Exec, delegating to ExecContext to integrate Hook logic.
func (db *DB) Exec(query string, args ...any) (sql.Result, error) {
	return db.ExecContext(context.Background(), query, args...)
}

// MustExec (panic) runs MustExec using this database.
// Any placeholder parameters are replaced with supplied args.
func (db *DB) MustExec(query string, args ...any) sql.Result {
	return MustExec(db, query, args...)
}

// Preparex returns an sqlex.Stmt instead of a sql.Stmt
func (db *DB) Preparex(query string) (*Stmt, error) {
	return Preparex(db, query)
}

// PrepareNamed returns an sqlex.NamedStmt
func (db *DB) PrepareNamed(query string) (*NamedStmt, error) {
	return prepareNamed(db, query)
}

// PrepareNamedContext returns an sqlex.NamedStmt
func (db *DB) PrepareNamedContext(ctx context.Context, query string) (*NamedStmt, error) {
	return prepareNamedContext(ctx, db, query)
}

// NamedQueryContext using this DB.
// Any named placeholder parameters are replaced with fields from arg.
func (db *DB) NamedQueryContext(ctx context.Context, query string, arg any) (*Rows, error) {
	return NamedQueryContext(ctx, db, query, arg)
}

// ExecContext executes a query without returning any rows.
// Overrides sql.DB's ExecContext to integrate Hook logic.
// Automatically detects slice args and expands IN clauses; zero overhead when no slices are present.
func (db *DB) ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error) {
	ctx, afterFunc, event, err := db.prepare(ctx, query, args, OpExec)
	if err != nil {
		return nil, err
	}
	result, err := db.DB.ExecContext(ctx, event.Query, event.Args...)
	event.Error = err
	if result != nil {
		event.RowsAffected, _ = result.RowsAffected()
		event.LastInsertID, _ = result.LastInsertId()
	}
	afterFunc()
	return result, err
}

// NamedExecContext using this DB.
// Any named placeholder parameters are replaced with fields from arg.
//
// Note: This method only does named-to-? conversion and forwarding; autoIn / Rebind / Hook
// are guaranteed by the downstream db.ExecContext implementation (see ExecerContext interface contract).
func (db *DB) NamedExecContext(ctx context.Context, query string, arg any) (sql.Result, error) {
	q, args, err := bindNamedMapper(QUESTION, query, arg, db.Mapper)
	if err != nil {
		return nil, err
	}
	return db.ExecContext(ctx, q, args...)
}

// SelectContext using this DB.
// Any placeholder parameters are replaced with supplied args.
func (db *DB) SelectContext(ctx context.Context, dest any, query string, args ...any) error {
	return SelectContext(ctx, db, dest, query, args...)
}

// GetContext using this DB.
// Any placeholder parameters are replaced with supplied args.
// An error is returned if the result set is empty.
func (db *DB) GetContext(ctx context.Context, dest any, query string, args ...any) error {
	return GetContext(ctx, db, dest, query, args...)
}

// PreparexContext returns an sqlex.Stmt instead of a sql.Stmt.
//
// The provided context is used for the preparation of the statement, not for
// the execution of the statement.
func (db *DB) PreparexContext(ctx context.Context, query string) (*Stmt, error) {
	return PreparexContext(ctx, db, query)
}

// QueryxContext queries the database and returns an *sqlex.Rows.
// Any placeholder parameters are replaced with supplied args.
// Automatically detects slice args and expands IN clauses; zero overhead when no slices are present.
func (db *DB) QueryxContext(ctx context.Context, query string, args ...any) (*Rows, error) {
	ctx, afterFunc, event, err := db.prepare(ctx, query, args, OpQuery)
	if err != nil {
		return nil, err
	}
	r, err := db.DB.QueryContext(ctx, event.Query, event.Args...)
	event.Error = err
	afterFunc()
	if err != nil {
		return nil, err
	}
	return &Rows{Rows: r, Mapper: db.Mapper, strict: db.strict}, nil
}

// QueryRowxContext queries the database and returns an *sqlex.Row.
// Any placeholder parameters are replaced with supplied args.
// Automatically detects slice args and expands IN clauses; zero overhead when no slices are present.
func (db *DB) QueryRowxContext(ctx context.Context, query string, args ...any) *Row {
	ctx, afterFunc, event, err := db.prepare(ctx, query, args, OpQuery)
	if err != nil {
		return &Row{err: err, Mapper: db.Mapper, strict: db.strict}
	}
	rows, qErr := db.DB.QueryContext(ctx, event.Query, event.Args...)
	event.Error = qErr
	afterFunc()
	return &Row{rows: rows, err: qErr, Mapper: db.Mapper, strict: db.strict}
}

// MustExecContext (panic) runs MustExec using this database.
// Any placeholder parameters are replaced with supplied args.
func (db *DB) MustExecContext(ctx context.Context, query string, args ...any) sql.Result {
	return MustExecContext(ctx, db, query, args...)
}

// Connx returns an *sqlex.Conn instead of an *sql.Conn.
func (db *DB) Connx(ctx context.Context) (*Conn, error) {
	conn, err := db.DB.Conn(ctx)
	if err != nil {
		return nil, err
	}

	return &Conn{Conn: conn, pipeline: db.pipeline, Mapper: db.Mapper, strict: db.strict}, nil
}
