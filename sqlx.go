package sqlex

import (
	"context"
	"database/sql"
	"reflect"
	"strings"
	"sync"

	"github.com/go-sqlex/sqlex/reflectx"
)

// Although the NameMapper is convenient, in practice it should not
// be relied on except for application code.  If you are writing a library
// that uses sqlex, you should be aware that the name mappings you expect
// can be overridden by your user's application.

// NameMapper is used to map column names to struct field names.  By default,
// it uses strings.ToLower to lowercase struct field names.  It can be set
// to whatever you want, but it is encouraged to be set before sqlex is used
// as name-to-field mappings are cached after first use on a type.
//
// Warning: NameMapper reads and writes are not concurrency-safe. It is recommended to set it
// only in an init() function; modifying it at runtime may cause data races. If you need different
// mapping strategies at runtime, use DB.MapperFunc() to set each DB instance individually.
var NameMapper = strings.ToLower
var origMapper = reflect.ValueOf(NameMapper)

// Rather than creating on init, this is created when necessary so that
// importers have time to customize the NameMapper.
var mpr *reflectx.Mapper

// mprMu protects mpr.
var mprMu sync.Mutex

// mapper returns a valid mapper using the configured NameMapper func.
func mapper() *reflectx.Mapper {
	mprMu.Lock()
	defer mprMu.Unlock()

	if mpr == nil {
		mpr = reflectx.NewMapperFunc("db", NameMapper)
	} else if origMapper != reflect.ValueOf(NameMapper) {
		// if NameMapper has changed, create a new mapper
		mpr = reflectx.NewMapperFunc("db", NameMapper)
		origMapper = reflect.ValueOf(NameMapper)
	}
	return mpr
}

// isScannable takes the reflect.Type and the actual dest value and returns
// whether or not it's Scannable.  Something is scannable if:
//   - it is not a struct
//   - it implements sql.Scanner
//   - it has no exported fields
func isScannable(t reflect.Type) bool {
	if reflect.PointerTo(t).Implements(_scannerInterface) {
		return true
	}
	if t.Kind() != reflect.Struct {
		return true
	}

	// it's not important that we use the right mapper for this particular object,
	// we're only concerned on how many exported fields this struct has
	return len(mapper().TypeMap(t).Index) == 0
}

var _scannerInterface = reflect.TypeOf((*sql.Scanner)(nil)).Elem()

// Queryer is an interface used by Get and Select.
//
// Implementer contract (must be upheld, otherwise cross-cutting concerns of Get/Select
// and other high-level methods will be broken):
//  1. Queryx / QueryRowx must perform autoIn internally (automatic IN slice expansion)
//  2. Queryx / QueryRowx must perform Rebind internally (driver bindvar conversion)
//  3. Queryx / QueryRowx must trigger the Hook chain
//
// sqlex.DB / Tx / Conn satisfy all contract requirements. The top-level Get/Select functions
// do not perform autoIn / Rebind / Hook themselves — they rely entirely on the Queryer
// implementation's guarantees. This is the design principle since Phase 2.0.
//
// Note: The standard library's Query method (returning *sql.Rows) is sqlx-compatible legacy
// and is not covered by this contract.
type Queryer interface {
	Query(query string, args ...any) (*sql.Rows, error)
	Queryx(query string, args ...any) (*Rows, error)
	QueryRowx(query string, args ...any) *Row
}

// Execer is an interface used by MustExec.
//
// Implementer contract (must be upheld, otherwise cross-cutting concerns of MustExec
// and other high-level methods will be broken):
//  1. Exec must perform autoIn internally (automatic IN slice expansion)
//  2. Exec must perform Rebind internally (driver bindvar conversion)
//  3. Exec must trigger the Hook chain
//
// sqlex.DB / Tx / Conn satisfy all contract requirements.
type Execer interface {
	Exec(query string, args ...any) (sql.Result, error)
}

// binder is an interface for something which can bind queries (Tx, DB)
type binder interface {
	DriverName() string
	Rebind(string) string
	BindNamed(string, any) (string, []any, error)
}

// Ext is a union interface which can bind, query, and exec, used by
// NamedQuery and NamedExec.
type Ext interface {
	binder
	Queryer
	Execer
}

// Preparer is an interface used by Preparex.
// It embeds the binder interface, enabling Preparex to automatically Rebind,
// allowing unified use of ? placeholders without worrying about underlying database differences.
type Preparer interface {
	Prepare(query string) (*sql.Stmt, error)
	binder
}

// QueryerContext is an interface used by GetContext and SelectContext.
//
// Implementer contract (must be upheld, otherwise cross-cutting concerns of GetContext/SelectContext
// and other high-level methods will be broken):
//  1. QueryxContext / QueryRowxContext must perform autoIn internally (automatic IN slice expansion)
//  2. QueryxContext / QueryRowxContext must perform Rebind internally (driver bindvar conversion)
//  3. QueryxContext / QueryRowxContext must trigger the Hook chain
//
// sqlex.DB / Tx / Conn satisfy all contract requirements.
//
// Note: The standard library's QueryContext method (returning *sql.Rows) is sqlx-compatible legacy
// and is not covered by this contract.
type QueryerContext interface {
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryxContext(ctx context.Context, query string, args ...any) (*Rows, error)
	QueryRowxContext(ctx context.Context, query string, args ...any) *Row
}

// PreparerContext is an interface used by PreparexContext.
// It embeds the binder interface, enabling PreparexContext to automatically Rebind,
// allowing unified use of ? placeholders without worrying about underlying database differences.
type PreparerContext interface {
	PrepareContext(ctx context.Context, query string) (*sql.Stmt, error)
	binder
}

// ExecerContext is an interface used by MustExecContext and LoadFileContext.
//
// Implementer contract (must be upheld, otherwise cross-cutting concerns of MustExecContext
// and other high-level methods will be broken):
//  1. ExecContext must perform autoIn internally (automatic IN slice expansion)
//  2. ExecContext must perform Rebind internally (driver bindvar conversion)
//  3. ExecContext must trigger the Hook chain
//
// sqlex.DB / Tx / Conn satisfy all contract requirements.
type ExecerContext interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}

// ExtContext is a union interface which can bind, query, and exec, with Context
// used by NamedQueryContext and NamedExecContext.
type ExtContext interface {
	binder
	QueryerContext
	ExecerContext
}

// mapperAccessor is an interface for getting the Mapper from an object.
type mapperAccessor interface {
	GetMapper() *reflectx.Mapper
}

// strictAccessor is an interface for getting the strict setting from an object.
type strictAccessor interface {
	IsStrict() bool
}

func mapperFor(i any) *reflectx.Mapper {
	if m, ok := i.(mapperAccessor); ok {
		return m.GetMapper()
	}
	return mapper()
}

// strictFor extracts the strict setting from an object.
// If the object does not support strict (e.g. a bare sql.DB), it defaults to false (lenient mode).
func strictFor(i any) bool {
	if s, ok := i.(strictAccessor); ok {
		return s.IsStrict()
	}
	return false
}

// NamedExt is a unified interface that keeps the named-parameter method signatures of DB, Tx,
// and Conn consistent, enabling generic data access functions that are agnostic to the execution
// context (DB / Tx / Conn). Symmetric with BindExt.
//
// Contract guarantees (all implementations must satisfy):
//  1. Automatic IN expansion: when a map/struct field contains a slice, IN (:field) is automatically
//     expanded to IN (?,?,...,?), consistent with BindExt behavior; zero overhead when no slices.
//  2. Automatic Rebind: callers use unified :name named placeholders; internally converted to the
//     target database's bindvar format (?/$N/:argN/@pN) without worrying about underlying differences.
//  3. Hook chain integration: all methods trigger BeforeQuery/AfterQuery hooks; the observable query
//     is the final SQL after IN expansion + Rebind (i.e. the SQL actually sent to the driver).
type NamedExt interface {
	// --- High-level convenience methods (struct scanning + automatic IN expansion) ---
	NamedGet(dest any, query string, param any) error
	NamedGetContext(ctx context.Context, dest any, query string, param any) error
	NamedSelect(dest any, query string, param any) error
	NamedSelectContext(ctx context.Context, dest any, query string, param any) error

	// --- Execution (INSERT/UPDATE/DELETE, with built-in IN expansion) ---
	NamedExec(query string, arg any) (sql.Result, error)
	NamedExecContext(ctx context.Context, query string, arg any) (sql.Result, error)

	// --- Low-level query primitives (with built-in IN expansion) ---
	NamedQuery(query string, arg any) (*Rows, error)
	NamedQueryContext(ctx context.Context, query string, arg any) (*Rows, error)
}

// BindExt is a unified interface that keeps the basic query method signatures of DB, Tx,
// and Conn consistent, enabling generic data access functions that are agnostic to the execution
// context (DB / Tx / Conn). This interface covers high-level convenience methods, execution,
// and low-level query primitives, unifying the capabilities of Queryer, Execer, QueryerContext,
// and ExecerContext.
//
// Contract guarantees (all implementations must satisfy):
//  1. Automatic IN expansion: when slice arguments are passed, IN (?) is automatically expanded
//     to IN (?,?,...,?), consistent with NamedExt behavior; zero overhead when no slices.
//     Applicable methods: Exec/ExecContext, Queryx/QueryxContext, QueryRowx/QueryRowxContext,
//     Select/SelectContext, Get/GetContext.
//  2. Automatic Rebind: callers use unified ? placeholders; internally converted to the target
//     database's bindvar format (?/$N/:argN/@pN) without worrying about underlying differences.
//  3. Hook chain integration: all methods trigger BeforeQuery/AfterQuery hooks; the observable query
//     is the final SQL after autoIn expansion + Rebind (i.e. the SQL actually sent to the driver).
type BindExt interface {
	// --- High-level convenience methods (struct scanning) ---
	Select(dest any, query string, args ...any) error
	SelectContext(ctx context.Context, dest any, query string, args ...any) error
	Get(dest any, query string, args ...any) error
	GetContext(ctx context.Context, dest any, query string, args ...any) error

	// --- Execution (INSERT/UPDATE/DELETE) ---
	Exec(query string, args ...any) (sql.Result, error)
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)

	// --- Low-level query primitives ---
	Queryx(query string, args ...any) (*Rows, error)
	QueryxContext(ctx context.Context, query string, args ...any) (*Rows, error)
	QueryRowx(query string, args ...any) *Row
	QueryRowxContext(ctx context.Context, query string, args ...any) *Row
}

// Compile-time check that DB, Tx, and Conn implement the NamedExt and BindExt interfaces.
// Although the underlying sql.Conn only has Context methods, the sqlex wrapper layer provides
// non-Context convenience methods (delegating to context.Background()), fully aligning the
// three types' interfaces and enabling generic data access functions like func doSomething(ext NamedExt).
var (
	_ NamedExt = (*DB)(nil)
	_ NamedExt = (*Tx)(nil)
	_ NamedExt = (*Conn)(nil)
	_ BindExt  = (*DB)(nil)
	_ BindExt  = (*Tx)(nil)
	_ BindExt  = (*Conn)(nil)
)

// Compile-time check that DB, Tx, and Conn implement the Preparer and PreparerContext interfaces.
var (
	_ Preparer        = (*DB)(nil)
	_ Preparer        = (*Tx)(nil)
	_ Preparer        = (*Conn)(nil)
	_ PreparerContext = (*DB)(nil)
	_ PreparerContext = (*Tx)(nil)
	_ PreparerContext = (*Conn)(nil)
)

var (
	// Compile-time check that DB, Tx, and Conn implement the Ext and ExtContext interfaces.
	_ Ext        = (*DB)(nil)
	_ Ext        = (*Tx)(nil)
	_ Ext        = (*Conn)(nil)
	_ ExtContext = (*DB)(nil)
	_ ExtContext = (*Tx)(nil)
	_ ExtContext = (*Conn)(nil)
)

// Compile-time check that DB, Tx, and Conn implement internal property access interfaces.
// These interfaces are used by factory functions like mapperFor / strictFor / hooksFor,
// ensuring that adding new types won't silently fall back to defaults due to missing implementations.
var (
	_ mapperAccessor = (*DB)(nil)
	_ mapperAccessor = (*Tx)(nil)
	_ mapperAccessor = (*Conn)(nil)
	_ strictAccessor = (*DB)(nil)
	_ strictAccessor = (*Tx)(nil)
	_ strictAccessor = (*Conn)(nil)
	_ hooksAccessor  = (*DB)(nil)
	_ hooksAccessor  = (*Tx)(nil)
	_ hooksAccessor  = (*Conn)(nil)
)

// --- Top-level convenience functions ---

// Connect to a database and verify with a ping.
func Connect(driverName, dataSourceName string, opts ...Opt) (*DB, error) {
	return ConnectContext(context.Background(), driverName, dataSourceName, opts...)
}

// ConnectContext to a database and verify with a ping.
func ConnectContext(ctx context.Context, driverName, dataSourceName string, opts ...Opt) (*DB, error) {
	db, err := Open(driverName, dataSourceName, opts...)
	if err != nil {
		return nil, err
	}
	err = db.PingContext(ctx)
	if err != nil {
		db.Close()
		return nil, err
	}
	return db, nil
}

// MustConnect connects to a database and panics on error.
func MustConnect(driverName, dataSourceName string, opts ...Opt) *DB {
	db, err := Connect(driverName, dataSourceName, opts...)
	if err != nil {
		panic(err)
	}
	return db
}

type hooksAccessor interface {
	getHooks() []Hook
}

// hooksFor extracts the hooks setting from an object.
func hooksFor(i any) []Hook {
	if h, ok := i.(hooksAccessor); ok {
		return h.getHooks()
	}
	return nil
}

// Preparex prepares a statement.
// Automatically converts ? placeholders to the target database's bindvar format (e.g. PostgreSQL's $N),
// consistent with other query methods, enabling unified ?-style SQL authoring.
func Preparex(p Preparer, query string) (*Stmt, error) {
	bound := Rebind(BindType(p.DriverName()), query)
	s, err := p.Prepare(bound)
	if err != nil {
		return nil, err
	}
	return &Stmt{Stmt: s, Mapper: mapperFor(p), hooks: hooksFor(p), query: bound, strict: strictFor(p)}, err
}

// PreparexContext prepares a statement.
// Automatically converts ? placeholders to the target database's bindvar format (e.g. PostgreSQL's $N),
// consistent with other query methods, enabling unified ?-style SQL authoring.
//
// The provided context is used for the preparation of the statement, not for
// the execution of the statement.
func PreparexContext(ctx context.Context, p PreparerContext, query string) (*Stmt, error) {
	bound := Rebind(BindType(p.DriverName()), query)
	s, err := p.PrepareContext(ctx, bound)
	if err != nil {
		return nil, err
	}
	return &Stmt{Stmt: s, Mapper: mapperFor(p), hooks: hooksFor(p), query: bound, strict: strictFor(p)}, err
}

// Select executes a query using the provided Queryer, and StructScans each row
// into dest, which must be a slice.  If the slice elements are scannable, then
// the result set must have only one column.  Otherwise, StructScan is used.
// The *sql.Rows are closed automatically.
// Any placeholder parameters are replaced with supplied args.
//
// Note: This function only does "scan dispatch"; it does not perform autoIn / Rebind / Hook —
// these cross-cutting concerns are guaranteed by the Queryer.Queryx implementation
// (see Queryer interface contract).
func Select(q Queryer, dest any, query string, args ...any) error {
	rows, err := q.Queryx(query, args...)
	if err != nil {
		return err
	}
	// if something happens here, we want to make sure the rows are Closed
	defer rows.Close()
	return scanAll(rows, dest, false)
}

// SelectContext executes a query using the provided Queryer, and StructScans
// each row into dest, which must be a slice.  If the slice elements are
// scannable, then the result set must have only one column.  Otherwise,
// StructScan is used. The *sql.Rows are closed automatically.
// Any placeholder parameters are replaced with supplied args.
//
// Note: This function only does "scan dispatch"; it does not perform autoIn / Rebind / Hook —
// these cross-cutting concerns are guaranteed by the QueryerContext.QueryxContext implementation
// (see QueryerContext interface contract).
func SelectContext(ctx context.Context, q QueryerContext, dest any, query string, args ...any) error {
	rows, err := q.QueryxContext(ctx, query, args...)
	if err != nil {
		return err
	}
	// if something happens here, we want to make sure the rows are Closed
	defer rows.Close()
	return scanAll(rows, dest, false)
}

// Get does a QueryRow using the provided Queryer, and scans the resulting row
// to dest.  If dest is scannable, the result must only have one column.  Otherwise,
// StructScan is used.  Get will return sql.ErrNoRows like row.Scan would.
// Any placeholder parameters are replaced with supplied args.
// An error is returned if the result set is empty.
//
// Note: This function only does "scan dispatch"; it does not perform autoIn / Rebind / Hook —
// these cross-cutting concerns are guaranteed by the Queryer.QueryRowx implementation
// (see Queryer interface contract).
func Get(q Queryer, dest any, query string, args ...any) error {
	r := q.QueryRowx(query, args...)
	return r.scanAny(dest, false)
}

// GetContext does a QueryRow using the provided Queryer, and scans the
// resulting row to dest.  If dest is scannable, the result must only have one
// column. Otherwise, StructScan is used.  Get will return sql.ErrNoRows like
// row.Scan would. Any placeholder parameters are replaced with supplied args.
// An error is returned if the result set is empty.
//
// Note: This function only does "scan dispatch"; it does not perform autoIn / Rebind / Hook —
// these cross-cutting concerns are guaranteed by the QueryerContext.QueryRowxContext implementation.
func GetContext(ctx context.Context, q QueryerContext, dest any, query string, args ...any) error {
	r := q.QueryRowxContext(ctx, query, args...)
	return r.scanAny(dest, false)
}

// MustExec execs the query using e and panics if there was an error.
// Any placeholder parameters are replaced with supplied args.
func MustExec(e Execer, query string, args ...any) sql.Result {
	res, err := e.Exec(query, args...)
	if err != nil {
		panic(err)
	}
	return res
}

// MustExecContext execs the query using e and panics if there was an error.
// Any placeholder parameters are replaced with supplied args.
func MustExecContext(ctx context.Context, e ExecerContext, query string, args ...any) sql.Result {
	res, err := e.ExecContext(ctx, query, args...)
	if err != nil {
		panic(err)
	}
	return res
}
